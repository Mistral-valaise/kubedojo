package observability

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/telemetry"
)

// TestIntegration runs the real stack in Docker and proves what the unit
// tests can only simulate. It needs Docker and pulls the image (0.9 GB), so
// it only runs when asked to:
//
//	KUBEDOJO_IT=1 go test -count=1 -timeout 15m -run Integration ./internal/observability/
//
// The stack has a name and ports of its own and never touches kubedojo-lgtm.
func TestIntegration(t *testing.T) {
	if os.Getenv("KUBEDOJO_IT") == "" {
		t.Skip("set KUBEDOJO_IT=1 to run the stack in Docker")
	}
	ctx := context.Background()
	docker := func(ctx context.Context, name string, args ...string) (string, error) {
		b, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
		return strings.TrimSpace(string(b)), err
	}
	freePort := func() int {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		return ln.Addr().(*net.TCPAddr).Port
	}
	opt := Options{Name: "kubedojo-lgtm-it", DataDir: t.TempDir(), GrafanaPort: freePort(), OTLPPort: freePort(), Version: "it"}
	stack := func(o Options) *Stack {
		s, err := New(o, docker)
		if err != nil {
			t.Fatal(err)
		}
		s.Out = testWriter{t}
		s.ReadyTimeout = 5 * time.Minute
		return s
	}
	s := stack(opt)
	t.Cleanup(func() {
		if err := s.Reset(context.Background()); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	id := func() string {
		o, err := docker(ctx, "docker", "inspect", "-f", "{{.Id}}", opt.Name)
		if err != nil {
			t.Fatalf("inspect: %s", o)
		}
		return o
	}
	verify := func(s *Stack) {
		t.Helper()
		for _, c := range s.Verify(ctx, 90*time.Second) {
			if c.Err != nil {
				t.Errorf("verify: %s: %v", c.Name, c.Err)
			} else {
				t.Logf("verify: ok: %s", c.Name)
			}
		}
	}

	// Start, and prove every signal and every link.
	start := time.Now()
	if err := s.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}
	t.Logf("the stack was ready after %s", time.Since(start).Round(time.Second))
	verify(s)
	first := id()

	// A second start keeps the container.
	if err := s.Up(ctx); err != nil {
		t.Fatalf("second Up: %v", err)
	}
	if id() != first {
		t.Error("a second Up replaced a stack that had the same settings")
	}

	// Nothing is published beyond this machine.
	ports, _ := docker(ctx, "docker", "port", opt.Name)
	for _, line := range strings.Split(ports, "\n") {
		if !strings.Contains(line, "-> 127.0.0.1:") {
			t.Errorf("a port is published beyond loopback: %s", line)
		}
	}

	// What was written is still there after the stack was replaced.
	traceID, err := telemetry.Canary(ctx, s.Endpoint())
	if err != nil {
		t.Fatalf("canary: %v", err)
	}
	trace := func(s *Stack) error {
		var tr struct {
			Batches []any `json:"batches"`
		}
		return s.eventually(ctx, 90*time.Second, func() error {
			if err := s.grafana(ctx, "/api/datasources/proxy/uid/tempo/api/traces/"+traceID, nil, &tr); err != nil {
				return err
			}
			if len(tr.Batches) == 0 {
				return fmt.Errorf("trace %s not found", traceID)
			}
			return nil
		})
	}
	if err := trace(s); err != nil {
		t.Fatalf("before the restart: %v", err)
	}
	// Other settings replace the container and keep the data.
	changed := opt
	changed.Retention = 48 * time.Hour
	s2 := stack(changed)
	if err := s2.Up(ctx); err != nil {
		t.Fatalf("Up with another retention: %v", err)
	}
	if id() == first {
		t.Error("Up kept a stack that was started with other settings")
	}
	if err := trace(s2); err != nil {
		t.Errorf("the trace written before the restart is gone: %v", err)
	}
	verify(s2)

	// A port somebody else holds is reported, and the stack keeps running.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	busy := changed
	busy.OTLPPort = ln.Addr().(*net.TCPAddr).Port
	before := id()
	if err := stack(busy).Up(ctx); err == nil || !strings.Contains(err.Error(), fmt.Sprint(busy.OTLPPort)) || !strings.Contains(err.Error(), "--otlp-port") {
		t.Errorf("Up on an occupied port = %v, want an error naming the port and the flag", err)
	}
	if id() != before {
		t.Error("the failed start took the running stack down")
	}

	// Stop keeps the data, Reset removes it.
	if removed, err := s2.Stop(ctx); err != nil || !removed {
		t.Fatalf("Stop = %v, %v", removed, err)
	}
	if st, err := s2.Status(ctx); err != nil || st.Exists || !st.Volume {
		t.Errorf("after Stop: %+v, %v; want no container and the volume", st, err)
	}
	if err := s2.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if st, err := s2.Status(ctx); err != nil || st.Exists || st.Volume {
		t.Errorf("after Reset: %+v, %v; want nothing left", st, err)
	}
	if _, err := os.Stat(filepath.Join(opt.DataDir, "observability")); !os.IsNotExist(err) {
		t.Errorf("the observability directory survived the reset: %v", err)
	}
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}
