package provisioning

import (
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type datasource struct {
	Name     string         `yaml:"name"`
	Type     string         `yaml:"type"`
	UID      string         `yaml:"uid"`
	URL      string         `yaml:"url"`
	JSONData map[string]any `yaml:"jsonData"`
}

func load(t *testing.T) []datasource {
	t.Helper()
	var f struct {
		APIVersion  int          `yaml:"apiVersion"`
		Datasources []datasource `yaml:"datasources"`
	}
	if err := yaml.Unmarshal(Datasources, &f); err != nil {
		t.Fatal(err)
	}
	if f.APIVersion != 1 {
		t.Fatalf("apiVersion %d", f.APIVersion)
	}
	return f.Datasources
}

// Dashboards and alert rules name the data sources by UID. The UIDs are those
// of grafana/otel-lgtm, so that its own dashboards keep working too.
func TestDatasourceUIDsAreStable(t *testing.T) {
	got := map[string]string{}
	for _, d := range load(t) {
		if d.UID == "" || d.URL == "" {
			t.Errorf("data source %q has no uid or url", d.Name)
		}
		got[d.UID] = d.Type
	}
	for uid, typ := range map[string]string{"prometheus": "prometheus", "tempo": "tempo", "loki": "loki", "pyroscope": "grafana-pyroscope-datasource"} {
		if got[uid] != typ {
			t.Errorf("data source %s has type %q, want %q", uid, got[uid], typ)
		}
	}
}

// refs collects every datasourceUid below v.
func refs(v any, out *[]string) {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if s, ok := val.(string); ok && k == "datasourceUid" {
				*out = append(*out, s)
			}
			refs(val, out)
		}
	case []any:
		for _, val := range x {
			refs(val, out)
		}
	}
}

// A link between signals that points at a data source which does not exist
// shows up in Grafana as a button that does nothing.
func TestCorrelationLinksPointAtExistingDatasources(t *testing.T) {
	ds := load(t)
	uids := map[string]bool{}
	by := map[string]datasource{}
	for _, d := range ds {
		uids[d.UID] = true
		by[d.UID] = d
	}
	for _, d := range ds {
		var r []string
		refs(d.JSONData, &r)
		for _, uid := range r {
			if !uids[uid] {
				t.Errorf("%s links to the data source %q, which is not defined", d.Name, uid)
			}
		}
	}
	// The four directions the local stack promises.
	for name, c := range map[string]struct{ from, key, to string }{
		"metrics → traces (exemplars)": {"prometheus", "exemplarTraceIdDestinations", "tempo"},
		"traces → logs":                {"tempo", "tracesToLogsV2", "loki"},
		"traces → metrics":             {"tempo", "tracesToMetrics", "prometheus"},
		"logs → traces":                {"loki", "derivedFields", "tempo"},
	} {
		var r []string
		refs(by[c.from].JSONData[c.key], &r)
		if len(r) == 0 || r[0] != c.to {
			t.Errorf("%s: %s.%s links to %v, want %s", name, c.from, c.key, r, c.to)
		}
	}
}

// Grafana expands $VAR and ${VAR} in provisioning files. A single $ in a
// query or link would be replaced by an empty string without any error.
func TestDollarSignsAreEscaped(t *testing.T) {
	single := regexp.MustCompile(`(^|[^$])\$([^$]|$)`)
	for i, line := range strings.Split(string(Datasources), "\n") {
		if single.MatchString(line) {
			t.Errorf("datasources.yaml:%d: a single $ is expanded by Grafana, write $$: %s", i+1, strings.TrimSpace(line))
		}
	}
}

func TestDashboardProviderReadsTheMountedDirectory(t *testing.T) {
	var f struct {
		Providers []struct {
			Options struct {
				Path string `yaml:"path"`
			} `yaml:"options"`
		} `yaml:"providers"`
	}
	if err := yaml.Unmarshal(DashboardProvider, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Providers) != 1 || f.Providers[0].Options.Path != DashboardsPath {
		t.Fatalf("the provider reads %+v, the dashboards are mounted at %s", f.Providers, DashboardsPath)
	}
}
