package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/kubedojo/kubedojo/internal/checker"
	"github.com/kubedojo/kubedojo/internal/k8shost"
	"github.com/kubedojo/kubedojo/internal/session"
	"github.com/kubedojo/kubedojo/internal/telemetry"
)

// telemetryHooks connects session events to the telemetry instruments.
func telemetryHooks(t *telemetry.Telemetry) session.Hooks {
	return session.Hooks{
		Provisioned: func(track, mode string, d time.Duration, err error) { t.Provisioned(track, mode, d, err) },
		Checked: func(track, question, mode string, res checker.Result) {
			t.Checked(track, question, mode, res.Score, res.Max)
		},
		ExamDone: func(track string, r session.Report) { t.ExamDone(track, r.Percent, r.Passed) },
	}
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
		p, err := k8shost.New(k8shost.Config{
			Namespace:    os.Getenv("KUBEDOJO_SANDBOX_NAMESPACE"),
			Image:        env("KUBEDOJO_SANDBOX_IMAGE", "ghcr.io/kubedojo/sandbox-host:"+version),
			CPURequest:   env("KUBEDOJO_SANDBOX_CPU_REQUEST", "1"),
			MemRequest:   env("KUBEDOJO_SANDBOX_MEMORY_REQUEST", "3Gi"),
			MemLimit:     env("KUBEDOJO_SANDBOX_MEMORY_LIMIT", "8Gi"),
			NodeSelector: sel,
			Toleration:   os.Getenv("KUBEDOJO_SANDBOX_TOLERATION"),
			PullSecret:   os.Getenv("KUBEDOJO_SANDBOX_PULL_SECRET"),
		})
		if err != nil {
			return nil, err
		}
		log.Info("sandboxes run as pods", "image", env("KUBEDOJO_SANDBOX_IMAGE", ""))
		return p, nil
	}
	return nil, fmt.Errorf("unknown sandbox host %q (use local or kubernetes)", kind)
}
