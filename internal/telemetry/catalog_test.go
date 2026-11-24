package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/trace/noop"
)

const catalogFile = "../../deploy/helm/kubedojo/observability/metrics.json"

// catalog is the list of metrics dashboards and alert rules may query. The
// linter in tools/obslint reads the same file.
type catalog struct {
	Kubedojo []family `json:"kubedojo"`
}

type family struct {
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Labels []string `json:"labels"`
}

// prometheusName is the name Prometheus gives an OTLP metric: dots become
// underscores, the unit is appended, and a counter ends in _total.
func prometheusName(name, unit, typ string) string {
	n := strings.ReplaceAll(name, ".", "_")
	if suffix := map[string]string{"s": "seconds", "%": "percent", "1": "ratio", "By": "bytes"}[unit]; suffix != "" && !strings.HasSuffix(n, "_"+suffix) {
		n += "_" + suffix
	}
	if typ == "counter" {
		n += "_total"
	}
	return n
}

func sets[N int64 | float64](points []metricdata.DataPoint[N]) []attribute.Set {
	out := make([]attribute.Set, len(points))
	for i, p := range points {
		out[i] = p.Attributes
	}
	return out
}

// The catalog is what kubedojo really exports. A renamed instrument or a new
// label must show up here, and through the linter in every dashboard and
// alert rule that queries it.
func TestMetricCatalogMatchesTheInstruments(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	tel := New(noop.NewTracerProvider(), sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)), slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()
	// Every instrument, at least once.
	tel.ObserveSessions(func() (int, int) { return 1, 1 })
	tel.ObserveCapacity(func() (int, int) { return 1, 3 })
	tel.ObserveSandboxes(func() (map[string]SandboxUsage, int64) {
		return map[string]SandboxUsage{"node": {Containers: 1, MemoryBytes: 1, CPU: 1}}, 1
	})
	tel.Provisioned(ctx, "cka", "practice", time.Second, nil)
	tel.Provisioned(ctx, "cka", "exam", time.Second, errors.New("x"))
	tel.Checked(ctx, "cka", "q", "practice", 1, 2)
	tel.ExamDone(ctx, "cka", 50, false)
	tel.Rated(ctx, "cka", "q", 4)
	tel.Terminal(1)
	tel.TerminalClosed(ctx, time.Minute)
	tel.StoreOperation("save_session", time.Millisecond)
	tel.Rejected("host")

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatal(err)
	}
	var got []family
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			f := family{}
			labels := map[string]bool{}
			add := func(sets ...attribute.Set) {
				for _, set := range sets {
					for _, kv := range set.ToSlice() {
						labels[string(kv.Key)] = true
					}
				}
			}
			switch d := m.Data.(type) {
			case metricdata.Gauge[int64]:
				f.Type = "gauge"
				add(sets(d.DataPoints)...)
			case metricdata.Gauge[float64]:
				f.Type = "gauge"
				add(sets(d.DataPoints)...)
			case metricdata.Sum[int64]:
				f.Type = "counter"
				add(sets(d.DataPoints)...)
			case metricdata.Histogram[float64]:
				f.Type = "histogram"
				for _, p := range d.DataPoints {
					add(p.Attributes)
				}
			case metricdata.Histogram[int64]:
				f.Type = "histogram"
				for _, p := range d.DataPoints {
					add(p.Attributes)
				}
			default:
				t.Fatalf("metric %s has data %T, which this test does not know", m.Name, m.Data)
			}
			f.Name = prometheusName(m.Name, m.Unit, f.Type)
			f.Labels = []string{}
			for k := range labels {
				f.Labels = append(f.Labels, k)
			}
			sort.Strings(f.Labels)
			got = append(got, f)
		}
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Name < got[j].Name })

	if os.Getenv("UPDATE_CATALOG") != "" {
		raw, err := os.ReadFile(catalogFile)
		if err != nil {
			t.Fatal(err)
		}
		var whole map[string]json.RawMessage
		if err := json.Unmarshal(raw, &whole); err != nil {
			t.Fatal(err)
		}
		whole["kubedojo"], _ = json.Marshal(got)
		out, _ := json.MarshalIndent(whole, "", "  ")
		if err := os.WriteFile(catalogFile, append(out, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	raw, err := os.ReadFile(catalogFile)
	if err != nil {
		t.Fatal(err)
	}
	var want catalog
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	a, _ := json.MarshalIndent(got, "", "  ")
	b, _ := json.MarshalIndent(want.Kubedojo, "", "  ")
	if string(a) != string(b) {
		t.Fatalf("the instruments and %s differ. Run UPDATE_CATALOG=1 go test ./internal/telemetry/ and review the dashboards and alert rules that use what changed.\n\nexported:\n%s\n\ncatalog:\n%s", catalogFile, a, b)
	}
	// Sessions and users never become labels: they are unbounded.
	for _, f := range got {
		for _, l := range f.Labels {
			if strings.Contains(l, "session") || strings.Contains(l, "user") || strings.Contains(l, "owner") {
				t.Errorf("%s has the label %q", f.Name, l)
			}
		}
	}
}
