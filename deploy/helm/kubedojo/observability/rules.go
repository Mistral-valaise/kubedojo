package provisioning

import (
	_ "embed"
	"fmt"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// RulesSource is rules.yaml: the alert rules, written once.
//
//go:embed rules.yaml
var RulesSource []byte

// GrafanaAlerts is grafana-alerts.yaml, generated from RulesSource: the rules
// of the single-container stack in Grafana's provisioning format.
//
//go:embed grafana-alerts.yaml
var GrafanaAlerts []byte

// GrafanaAlertsPath is where grafana-alerts.yaml belongs in the container.
const GrafanaAlertsPath = "/otel-lgtm/grafana/conf/provisioning/alerting/kubedojo.yaml"

// Scopes of a rule: where it can be evaluated.
const (
	ScopeLocal      = "local"      // the single-container stack
	ScopeKubernetes = "kubernetes" // a Prometheus or Mimir fed by the chart
)

// Rule is one alert rule of rules.yaml.
type Rule struct {
	Name        string   `yaml:"name"`
	Scope       []string `yaml:"scope"`
	Expr        string   `yaml:"expr"`
	For         string   `yaml:"for"`
	LocalFor    string   `yaml:"localFor"`
	Severity    string   `yaml:"severity"`
	Summary     string   `yaml:"summary"`
	Description string   `yaml:"description"`
}

// Rules parses rules.yaml and checks what a generator cannot repair.
func Rules(src []byte) ([]Rule, error) {
	var f struct {
		Rules []Rule `yaml:"rules"`
	}
	if err := yaml.Unmarshal(src, &f); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, r := range f.Rules {
		switch {
		case r.Name == "" || !strings.HasPrefix(r.Name, "Kubedojo"):
			return nil, fmt.Errorf("rule %q: the name must start with Kubedojo", r.Name)
		case seen[r.Name]:
			return nil, fmt.Errorf("rule %s is defined twice", r.Name)
		case strings.TrimSpace(r.Expr) == "" || r.Summary == "" || r.Description == "":
			return nil, fmt.Errorf("rule %s needs expr, summary and description", r.Name)
		case !slices.Contains([]string{"critical", "warning", "info"}, r.Severity):
			return nil, fmt.Errorf("rule %s: severity %q is not critical, warning or info", r.Name, r.Severity)
		case len(r.Scope) == 0:
			return nil, fmt.Errorf("rule %s has no scope", r.Name)
		}
		seen[r.Name] = true
		for _, s := range r.Scope {
			if s != ScopeLocal && s != ScopeKubernetes {
				return nil, fmt.Errorf("rule %s: unknown scope %q", r.Name, s)
			}
		}
		if _, err := time.ParseDuration(r.For); err != nil {
			return nil, fmt.Errorf("rule %s: for %q is not a duration", r.Name, r.For)
		}
		if r.LocalFor != "" {
			if _, err := time.ParseDuration(r.LocalFor); err != nil {
				return nil, fmt.Errorf("rule %s: localFor %q is not a duration", r.Name, r.LocalFor)
			}
		}
		// Helm renders the texts as templates, Grafana expands $ in the file.
		if all := r.Expr + r.Summary + r.Description; strings.Contains(all, "{{") || strings.Contains(all, "$") {
			return nil, fmt.Errorf("rule %s: no templates and no dollar signs, please", r.Name)
		}
	}
	return f.Rules, nil
}

// uid is the stable identifier of a rule in Grafana: KubedojoDown → kubedojo-down.
func uid(name string) string {
	var b strings.Builder
	for i, r := range name {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('-')
			}
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}

// GenerateGrafanaAlerts renders the rules with scope local as a Grafana
// alerting provisioning file.
//
// A PromQL condition returns series while it holds and nothing otherwise.
// Grafana needs a number to compare, so each rule counts those series: more
// than zero is firing, and no data is the normal state.
//
// The file provisions no contact point and no notification policy. Alerts go
// to Grafana's default receiver, an e-mail contact point without a mail
// server in this image: they are visible in Grafana and nothing is sent.
func GenerateGrafanaAlerts(rules []Rule) ([]byte, error) {
	type m = map[string]any
	var out []m
	for _, r := range rules {
		if !slices.Contains(r.Scope, ScopeLocal) {
			continue
		}
		wait := r.For
		if r.LocalFor != "" {
			wait = r.LocalFor
		}
		out = append(out, m{
			"uid":          uid(r.Name),
			"title":        r.Name,
			"condition":    "C",
			"for":          wait,
			"noDataState":  "OK",
			"execErrState": "Error",
			"labels":       m{"severity": r.Severity},
			"annotations":  m{"summary": r.Summary, "description": r.Description},
			"data": []m{
				{
					"refId":             "A",
					"datasourceUid":     "prometheus",
					"relativeTimeRange": m{"from": 600, "to": 0},
					"model": m{
						"refId":   "A",
						"expr":    "count(\n" + strings.TrimSpace(r.Expr) + "\n)",
						"instant": true,
					},
				},
				{
					"refId":         "C",
					"datasourceUid": "__expr__",
					"model": m{
						"refId":      "C",
						"type":       "threshold",
						"expression": "A",
						"conditions": []m{{"evaluator": m{"type": "gt", "params": []int{0}}}},
					},
				},
			},
		})
	}
	doc := m{
		"apiVersion": 1,
		"groups": []m{{
			"orgId":    1,
			"name":     "kubedojo",
			"folder":   "kubedojo",
			"interval": "1m",
			"rules":    out,
		}},
	}
	body, err := yaml.Marshal(doc)
	if err != nil {
		return nil, err
	}
	const head = "# Generated from rules.yaml. Do not edit:\n#   UPDATE_ALERTS=1 go test ./deploy/helm/kubedojo/observability/\n"
	return append([]byte(head), body...), nil
}
