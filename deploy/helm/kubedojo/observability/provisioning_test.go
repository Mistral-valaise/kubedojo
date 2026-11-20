package provisioning

import (
	"os"
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

// grafana-alerts.yaml is generated. This fails when rules.yaml changed and
// the file was not regenerated.
func TestGrafanaAlertsAreGeneratedFromTheRules(t *testing.T) {
	rules, err := Rules(RulesSource)
	if err != nil {
		t.Fatal(err)
	}
	want, err := GenerateGrafanaAlerts(rules)
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("UPDATE_ALERTS") != "" {
		if err := os.WriteFile("grafana-alerts.yaml", want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if string(GrafanaAlerts) != string(want) {
		t.Fatal("grafana-alerts.yaml is not what rules.yaml generates. Run: UPDATE_ALERTS=1 go test ./deploy/helm/kubedojo/observability/")
	}
}

func TestGrafanaAlerts(t *testing.T) {
	rules, err := Rules(RulesSource)
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Groups []struct {
			Folder string `yaml:"folder"`
			Rules  []struct {
				UID         string `yaml:"uid"`
				Title       string `yaml:"title"`
				For         string `yaml:"for"`
				NoDataState string `yaml:"noDataState"`
				Data        []struct {
					DatasourceUID string `yaml:"datasourceUid"`
				} `yaml:"data"`
			} `yaml:"rules"`
		} `yaml:"groups"`
		ContactPoints []any `yaml:"contactPoints"`
		Policies      []any `yaml:"policies"`
	}
	if err := yaml.Unmarshal(GrafanaAlerts, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Groups) != 1 || f.Groups[0].Folder != "kubedojo" {
		t.Fatalf("groups: %+v", f.Groups)
	}
	got := map[string]string{}
	uids := map[string]bool{}
	for _, d := range load(t) {
		uids[d.UID] = true
	}
	for _, r := range f.Groups[0].Rules {
		got[r.Title] = r.For
		if !strings.HasPrefix(r.UID, "kubedojo-") || len(r.UID) > 40 {
			t.Errorf("%s: uid %q", r.Title, r.UID)
		}
		// A condition that does not hold returns nothing. That is the good
		// state, not a reason to alert.
		if r.NoDataState != "OK" {
			t.Errorf("%s: noDataState %q fires whenever the condition is false", r.Title, r.NoDataState)
		}
		if len(r.Data) == 0 || !uids[r.Data[0].DatasourceUID] {
			t.Errorf("%s queries a data source that is not provisioned: %+v", r.Title, r.Data)
		}
	}
	for _, r := range rules {
		local := false
		for _, s := range r.Scope {
			local = local || s == ScopeLocal
		}
		wait, ok := got[r.Name]
		if ok != local {
			t.Errorf("%s: in grafana-alerts.yaml %v, scope local %v", r.Name, ok, local)
		}
		if local && r.LocalFor != "" && wait != r.LocalFor {
			t.Errorf("%s waits %s locally, want %s", r.Name, wait, r.LocalFor)
		}
	}
	// What the local stack promises to watch.
	for _, name := range []string{"KubedojoDown", "KubedojoHighErrorRate", "KubedojoSandboxProvisioningFails",
		"KubedojoSandboxProvisioningSlow", "KubedojoSessionLimitReached", "KubedojoSandboxMemoryPressure"} {
		if _, ok := got[name]; !ok {
			t.Errorf("the local stack has no rule %s", name)
		}
	}
	// No notification is configured: the alerts are visible in Grafana only.
	if len(f.ContactPoints) != 0 || len(f.Policies) != 0 {
		t.Error("the file provisions contact points or policies; alerts must not leave the machine by default")
	}
	if strings.Contains(string(GrafanaAlerts), "$") {
		t.Error("a dollar sign in grafana-alerts.yaml: Grafana would expand it")
	}
}

func TestRulesAreValidated(t *testing.T) {
	const ok = "name: KubedojoX\nscope: [local]\nexpr: up == 0\nfor: 5m\nseverity: warning\nsummary: s\ndescription: d\n"
	rule := func(replace, with string) []byte {
		body := strings.Replace(ok, replace, with, 1)
		return []byte("rules:\n  - " + strings.ReplaceAll(strings.TrimSpace(body), "\n", "\n    ") + "\n")
	}
	if _, err := Rules(rule("", "")); err != nil {
		t.Fatalf("a valid rule was rejected: %v", err)
	}
	for name, src := range map[string][]byte{
		"name":      rule("KubedojoX", "Other"),
		"scope":     rule("[local]", "[moon]"),
		"no scope":  rule("scope: [local]\n", ""),
		"duration":  rule("5m", "soon"),
		"severity":  rule("warning", "panic"),
		"template":  rule("summary: s", "summary: '{{ $value }}'"),
		"no expr":   rule("expr: up == 0\n", ""),
		"duplicate": append(rule("", ""), []byte("  - "+strings.ReplaceAll(strings.TrimSpace(ok), "\n", "\n    ")+"\n")...),
	} {
		if _, err := Rules(src); err == nil {
			t.Errorf("%s: an invalid rule was accepted", name)
		}
	}
}
