// Package obslint checks kubedojo's Grafana dashboards without a Grafana: the
// queries must parse, and they may only ask for metrics that exist where the
// dashboard is installed.
//
// It is a module of its own so that the Prometheus parser, with everything it
// depends on, stays out of kubedojo's go.mod. Run it with
//
//	make lint-dashboards
//
// which is `go test -count=1`: Go does not notice changes to files outside a
// module, so a cached result would hide a broken dashboard.
package obslint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prometheus/prometheus/promql/parser"
	"gopkg.in/yaml.v3"
)

const chart = "../../deploy/helm/kubedojo"

type catalog struct {
	Kubedojo []struct {
		Name   string   `json:"name"`
		Type   string   `json:"type"`
		Labels []string `json:"labels"`
	} `json:"kubedojo"`
	LocalOnly []string `json:"localOnly"`
	External  []struct {
		Name   string   `json:"name"`
		Prefix string   `json:"prefix"`
		Type   string   `json:"type"`
		Where  []string `json:"where"`
	} `json:"external"`
}

func loadCatalog(t *testing.T) catalog {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(chart, "observability", "metrics.json"))
	if err != nil {
		t.Fatal(err)
	}
	var c catalog
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	if len(c.Kubedojo) == 0 {
		t.Fatal("the catalog lists no kubedojo metrics")
	}
	return c
}

// series are the names a metric family has in Prometheus.
func series(name, typ string) []string {
	if typ == "histogram" {
		return []string{name + "_bucket", name + "_sum", name + "_count"}
	}
	return []string{name}
}

// allowed reports whether a dashboard of a scope may query a metric, and the
// labels it has if they are known.
func (c catalog) allowed(metric, scope string) (ok bool, labels []string, why string) {
	local := map[string]bool{}
	for _, n := range c.LocalOnly {
		local[n] = true
	}
	for _, f := range c.Kubedojo {
		for _, s := range series(f.Name, f.Type) {
			if s != metric {
				continue
			}
			if local[f.Name] && scope != "local" {
				return false, nil, "it only exists in a local kubedojo, where sandboxes are Docker containers"
			}
			return true, f.Labels, ""
		}
	}
	for _, e := range c.External {
		match := e.Prefix != "" && strings.HasPrefix(metric, e.Prefix)
		for _, s := range series(e.Name, e.Type) {
			match = match || (e.Name != "" && s == metric)
		}
		if !match {
			continue
		}
		for _, w := range e.Where {
			if w == scope {
				return true, nil, ""
			}
		}
		return false, nil, "it is only available to the dashboards in " + strings.Join(e.Where, ", ")
	}
	return false, nil, "it is not in observability/metrics.json: kubedojo does not export it"
}

type dashboard struct {
	file  string
	scope string // shared, local, lgtm or kubernetes
	UID   string `json:"uid"`
	Title string `json:"title"`
	Tags  []string
	// Templating holds the data source variables.
	Templating struct {
		List []struct {
			Name  string `json:"name"`
			Type  string `json:"type"`
			Query string `json:"query"`
		} `json:"list"`
	} `json:"templating"`
	Panels []panel `json:"panels"`
}

type datasource struct {
	Type string `json:"type"`
	UID  string `json:"uid"`
}

type panel struct {
	ID         int         `json:"id"`
	Type       string      `json:"type"`
	Title      string      `json:"title"`
	Datasource *datasource `json:"datasource"`
	Targets    []struct {
		Datasource *datasource `json:"datasource"`
		Expr       string      `json:"expr"`
		Query      string      `json:"query"`
		Exemplar   bool        `json:"exemplar"`
	} `json:"targets"`
}

func loadDashboards(t *testing.T) []dashboard {
	t.Helper()
	var out []dashboard
	for scope, pattern := range map[string]string{"shared": "*.json", "local": "local/*.json", "lgtm": "lgtm/*.json", "kubernetes": "kubernetes/*.json"} {
		files, err := filepath.Glob(filepath.Join(chart, "dashboards", pattern))
		if err != nil {
			t.Fatal(err)
		}
		if len(files) == 0 {
			t.Fatalf("no dashboards match %s", pattern)
		}
		for _, f := range files {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			d := dashboard{file: strings.TrimPrefix(f, chart+"/"), scope: scope}
			if err := json.Unmarshal(raw, &d); err != nil {
				t.Fatalf("%s: %v", d.file, err)
			}
			out = append(out, d)
		}
	}
	return out
}

// promQL replaces Grafana's variables by values the parser accepts.
func promQL(expr string) string {
	for _, v := range []string{"$__rate_interval", "$__interval", "$__range"} {
		expr = strings.ReplaceAll(expr, v, "5m")
	}
	return expr
}

func TestDashboardQueries(t *testing.T) {
	cat := loadCatalog(t)
	for _, d := range loadDashboards(t) {
		for _, p := range d.Panels {
			for _, tg := range p.Targets {
				ds := tg.Datasource
				if ds == nil {
					ds = p.Datasource
				}
				where := d.file + ", panel " + p.Title
				if ds == nil {
					t.Errorf("%s: a query without a data source", where)
					continue
				}
				if ds.Type != "prometheus" {
					continue // LogQL and TraceQL are checked against the running stack
				}
				if tg.Expr == "" {
					t.Errorf("%s: an empty query", where)
					continue
				}
				// increase() and rate() over $__interval lose samples as soon as
				// the interval is shorter than the export interval.
				if strings.Contains(tg.Expr, "[$__interval]") {
					t.Errorf("%s: use [$__rate_interval], not [$__interval]: %s", where, tg.Expr)
				}
				expr, err := parser.NewParser(parser.Options{}).ParseExpr(promQL(tg.Expr))
				if err != nil {
					t.Errorf("%s: the query does not parse: %v\n  %s", where, err, tg.Expr)
					continue
				}
				parser.Inspect(expr, func(n parser.Node, _ []parser.Node) error {
					vs, ok := n.(*parser.VectorSelector)
					if !ok {
						return nil
					}
					ok, labels, why := cat.allowed(vs.Name, d.scope)
					if !ok {
						t.Errorf("%s: the metric %s cannot be used here: %s", where, vs.Name, why)
						return nil
					}
					if labels == nil {
						return nil
					}
					// Labels kubedojo sets, plus the ones Prometheus adds.
					known := map[string]bool{"__name__": true, "le": true, "job": true, "instance": true,
						"service_name": true, "service_version": true, "host_name": true}
					for _, l := range labels {
						known[l] = true
					}
					for _, m := range vs.LabelMatchers {
						if !known[m.Name] {
							t.Errorf("%s: %s has no label %q (it has %v)", where, vs.Name, m.Name, labels)
						}
					}
					return nil
				})
			}
		}
	}
}

func TestDashboardStructure(t *testing.T) {
	uids := map[string]string{}
	for _, d := range loadDashboards(t) {
		if d.UID == "" || !strings.HasPrefix(d.UID, "kubedojo-") {
			t.Errorf("%s: the uid %q must start with kubedojo-", d.file, d.UID)
		}
		if other, dup := uids[d.UID]; dup {
			t.Errorf("%s and %s share the uid %s", d.file, other, d.UID)
		}
		uids[d.UID] = d.file
		if !strings.HasPrefix(d.Title, "kubedojo / ") {
			t.Errorf("%s: the title %q must start with \"kubedojo / \"", d.file, d.Title)
		}
		tagged := false
		for _, tag := range d.Tags {
			tagged = tagged || tag == "kubedojo"
		}
		if !tagged {
			t.Errorf("%s: no kubedojo tag, so the links between the dashboards miss it", d.file)
		}
		// Data sources are variables: the UIDs differ from one Grafana to the next.
		vars := map[string]string{}
		for _, v := range d.Templating.List {
			if v.Type == "datasource" {
				vars["${"+v.Name+"}"] = v.Query
			}
		}
		ids := map[int]bool{}
		for _, p := range d.Panels {
			if ids[p.ID] {
				t.Errorf("%s: two panels have the id %d", d.file, p.ID)
			}
			ids[p.ID] = true
			check := func(ds *datasource) {
				if ds == nil {
					return
				}
				kind, ok := vars[ds.UID]
				if !ok {
					t.Errorf("%s, panel %s: the data source %q is not a variable of the dashboard", d.file, p.Title, ds.UID)
				} else if kind != ds.Type {
					t.Errorf("%s, panel %s: the variable %s selects %s data sources, the panel needs %s", d.file, p.Title, ds.UID, kind, ds.Type)
				}
			}
			check(p.Datasource)
			for _, tg := range p.Targets {
				check(tg.Datasource)
			}
		}
	}
}

// The panels that promise a way from a metric to a trace must ask for exemplars.
func TestExemplarsAreRequestedWhereTheyArePromised(t *testing.T) {
	for _, d := range loadDashboards(t) {
		for _, p := range d.Panels {
			for _, tg := range p.Targets {
				histogram := strings.Contains(tg.Expr, "http_server_request_duration_seconds_bucket") ||
					strings.Contains(tg.Expr, "kubedojo_sandbox_provision_duration_seconds_bucket")
				if p.Type == "timeseries" && histogram && !strings.Contains(tg.Expr, "topk") && !tg.Exemplar {
					t.Errorf("%s, panel %s: a latency series without exemplars; set \"exemplar\": true", d.file, p.Title)
				}
			}
		}
	}
}

type rule struct {
	Name  string   `yaml:"name"`
	Scope []string `yaml:"scope"`
	Expr  string   `yaml:"expr"`
}

// Alert rules are queries too. A rule is evaluated in every scope it names,
// so its metrics must exist in each of them.
func TestAlertRuleQueries(t *testing.T) {
	cat := loadCatalog(t)
	raw, err := os.ReadFile(filepath.Join(chart, "observability", "rules.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Rules []rule `yaml:"rules"`
	}
	if err := yaml.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Rules) == 0 {
		t.Fatal("rules.yaml has no rules")
	}
	for _, r := range f.Rules {
		expr, err := parser.NewParser(parser.Options{}).ParseExpr(r.Expr)
		if err != nil {
			t.Errorf("rule %s does not parse: %v", r.Name, err)
			continue
		}
		parser.Inspect(expr, func(n parser.Node, _ []parser.Node) error {
			vs, ok := n.(*parser.VectorSelector)
			if !ok {
				return nil
			}
			for _, scope := range r.Scope {
				ok, _, why := cat.allowed(vs.Name, scope)
				// The single-container stack also has its collector's metrics.
				if !ok && scope == "local" {
					ok, _, _ = cat.allowed(vs.Name, "lgtm")
				}
				if !ok {
					t.Errorf("rule %s, scope %s: the metric %s cannot be used: %s", r.Name, scope, vs.Name, why)
				}
			}
			return nil
		})
	}
}
