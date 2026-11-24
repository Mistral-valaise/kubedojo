package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/checker"
	"github.com/Mistral-valaise/kubedojo/internal/dockerhost"
	"github.com/Mistral-valaise/kubedojo/internal/k8shost"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox"
	"github.com/Mistral-valaise/kubedojo/internal/session"
	"github.com/Mistral-valaise/kubedojo/internal/telemetry"
)

// telemetryHooks connects session events to the telemetry instruments. The
// store is observed through them as well: the session manager hands
// StoreOperation to the store it is given.
func telemetryHooks(t *telemetry.Telemetry) session.Hooks {
	return session.Hooks{
		Provisioned: t.Provisioned,
		Checked: func(ctx context.Context, track, question, mode string, res checker.Result) {
			t.Checked(ctx, track, question, mode, res.Score, res.Max)
		},
		ExamDone: func(ctx context.Context, track string, r session.Report) {
			t.ExamDone(ctx, track, r.Percent, r.Passed)
		},
		Tracer:         t,
		Detach:         telemetry.Detach,
		TerminalClosed: t.TerminalClosed,
		StoreOperation: t.StoreOperation,
	}
}

// envOrEmpty is env for a setting whose empty value means something: a limit
// that is set to nothing is no limit, a limit that is not set is the default.
func envOrEmpty(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

// hostProvider selects where sandboxes run.
func hostProvider(ctx context.Context, kind string, log *slog.Logger) (session.HostProvider, error) {
	switch kind {
	case "local":
		return session.LocalHosts(), nil
	case "kubernetes":
		sel := map[string]string{}
		for _, kv := range strings.Split(os.Getenv("KUBEDOJO_SANDBOX_NODE_SELECTOR"), ",") {
			if k, v, ok := strings.Cut(strings.TrimSpace(kv), "="); ok {
				sel[k] = v
			}
		}
		image := env("KUBEDOJO_SANDBOX_IMAGE", "ghcr.io/mistral-valaise/kubedojo-sandbox-host:v"+strings.TrimPrefix(version, "v"))
		p, err := k8shost.New(k8shost.Config{
			Namespace: os.Getenv("KUBEDOJO_SANDBOX_NAMESPACE"),
			Image:     image,
			// The defaults fit one cluster with room for a second, as an exam
			// has; see docs/deployment.md for sizing.
			CPURequest:       env("KUBEDOJO_SANDBOX_CPU_REQUEST", "1"),
			CPULimit:         envOrEmpty("KUBEDOJO_SANDBOX_CPU_LIMIT", "4"),
			MemRequest:       env("KUBEDOJO_SANDBOX_MEMORY_REQUEST", "3Gi"),
			MemLimit:         envOrEmpty("KUBEDOJO_SANDBOX_MEMORY_LIMIT", "8Gi"),
			EphemeralRequest: env("KUBEDOJO_SANDBOX_EPHEMERAL_REQUEST", "10Gi"),
			EphemeralLimit:   envOrEmpty("KUBEDOJO_SANDBOX_EPHEMERAL_LIMIT", "30Gi"),
			NodeSelector:     sel,
			Toleration:       os.Getenv("KUBEDOJO_SANDBOX_TOLERATION"),
			PullSecret:       os.Getenv("KUBEDOJO_SANDBOX_PULL_SECRET"),
			// Set by the chart; without them sandboxes are neither owned by
			// the installation nor told apart from another one's.
			Instance:        os.Getenv("KUBEDOJO_INSTANCE"),
			OwnerDeployment: os.Getenv("KUBEDOJO_OWNER_DEPLOYMENT"),
			StartTimeout:    envDuration("KUBEDOJO_SANDBOX_START_TIMEOUT", 0),
		})
		if err != nil {
			return nil, err
		}
		if err := p.AdoptOwner(ctx); err != nil {
			log.Warn("sandbox Pods will not be removed with the installation", "err", err)
		}
		log.Info("sandboxes run as pods", "image", image, "namespace", p.Namespace())
		return p, nil
	}
	return nil, fmt.Errorf("unknown sandbox host %q (use local or kubernetes)", kind)
}

// sampleSandboxes measures the sandbox containers of a local kubedojo every
// 30 seconds until ctx ends, and returns a function that reports the last
// measurement. Measuring takes `docker stats` about two seconds, too long for
// the callback of a metric.
func sampleSandboxes(ctx context.Context, drv *sandbox.Kind, log *slog.Logger) func() (map[string]telemetry.SandboxUsage, int64) {
	var mu sync.Mutex
	last, total := map[string]telemetry.SandboxUsage{}, int64(0)
	measure := func() {
		mctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		usage, mem, err := drv.Usage(mctx, dockerhost.Local())
		if err != nil {
			if ctx.Err() == nil {
				log.Debug("sandbox usage", "err", err)
			}
			return
		}
		next := map[string]telemetry.SandboxUsage{}
		for role, u := range usage {
			next[role] = telemetry.SandboxUsage{Containers: u.Containers, MemoryBytes: u.MemoryBytes, CPU: u.CPU}
		}
		mu.Lock()
		last, total = next, mem
		mu.Unlock()
	}
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for measure(); ; measure() {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	return func() (map[string]telemetry.SandboxUsage, int64) {
		mu.Lock()
		defer mu.Unlock()
		return last, total
	}
}
