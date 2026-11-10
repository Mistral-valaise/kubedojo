package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/kubedojo/kubedojo/deploy/helm/kubedojo/dashboards"
)

const lgtmContainer = "kubedojo-lgtm"

// startObservability runs Grafana's all-in-one LGTM container (Loki, Grafana,
// Tempo, Mimir-compatible Prometheus, OTel Collector) next to kubedojo, loads
// the same dashboards the Helm chart ships, and points the OTLP exporters at it.
func startObservability(ctx context.Context, dataDir string) error {
	dir := filepath.Join(dataDir, "observability")
	dash := filepath.Join(dir, "dashboards")
	if err := os.MkdirAll(dash, 0o755); err != nil {
		return err
	}
	entries, err := fs.Glob(dashboards.FS, "*.json")
	if err != nil {
		return err
	}
	for _, name := range entries {
		b, _ := dashboards.FS.ReadFile(name)
		if err := os.WriteFile(filepath.Join(dash, name), b, 0o644); err != nil {
			return err
		}
	}
	prov := filepath.Join(dir, "kubedojo.yaml")
	if err := os.WriteFile(prov, []byte("apiVersion: 1\nproviders:\n  - name: kubedojo\n    folder: kubedojo\n    type: file\n    options:\n      path: /otel-lgtm/kubedojo-dashboards\n"), 0o644); err != nil {
		return err
	}
	if state, _ := out(ctx, "docker", "inspect", "-f", "{{.State.Running}}", lgtmContainer); strings.TrimSpace(state) != "true" {
		_, _ = out(ctx, "docker", "rm", "-f", lgtmContainer)
		fmt.Println("  … starting the local observability stack (grafana/otel-lgtm)")
		if o, err := out(ctx, "docker", "run", "-d", "--name", lgtmContainer,
			"-p", "127.0.0.1:3000:3000", "-p", "127.0.0.1:4318:4318",
			"-v", dash+":/otel-lgtm/kubedojo-dashboards:ro",
			"-v", prov+":/otel-lgtm/grafana/conf/provisioning/dashboards/kubedojo.yaml:ro",
			"grafana/otel-lgtm:latest"); err != nil {
			return fmt.Errorf("%s", o)
		}
	}
	os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318")
	fmt.Println("  ✓ Grafana: http://localhost:3000/dashboards?tag=kubedojo  (stop with: docker rm -f " + lgtmContainer + ")")
	return nil
}
