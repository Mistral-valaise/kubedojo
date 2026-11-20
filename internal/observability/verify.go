package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Mistral-valaise/kubedojo/deploy/helm/kubedojo/dashboards"
	provisioning "github.com/Mistral-valaise/kubedojo/deploy/helm/kubedojo/observability"
	"github.com/Mistral-valaise/kubedojo/internal/telemetry"
)

// Check is one thing Verify looked at.
type Check struct {
	Name string
	// Err is nil when the check passed.
	Err error
}

// Verify proves that the running stack works end to end, through nothing but
// its two published ports: it sends a span, a log record and a metric sample
// to the OTLP receiver and reads all three back through Grafana, follows the
// links between them, and looks for the provisioned data sources, dashboards
// and alert rules. Reading back can take a few seconds; wait bounds it.
func (s *Stack) Verify(ctx context.Context, wait time.Duration) []Check {
	var checks []Check
	add := func(name string, err error) { checks = append(checks, Check{name, err}) }

	// What Grafana was given at start.
	var sources []struct {
		UID string `json:"uid"`
	}
	err := s.grafana(ctx, "/api/datasources", nil, &sources)
	if err == nil {
		have := map[string]bool{}
		for _, d := range sources {
			have[d.UID] = true
		}
		for _, uid := range []string{"prometheus", "loki", "tempo"} {
			if !have[uid] {
				err = fmt.Errorf("the data source %s is missing", uid)
			}
		}
	}
	add("Grafana has the Prometheus, Loki and Tempo data sources", err)

	want := 0
	for _, pattern := range dashboards.Local {
		m, _ := fs.Glob(dashboards.FS, pattern)
		want += len(m)
	}
	var found []struct {
		UID string `json:"uid"`
	}
	err = s.grafana(ctx, "/api/search", url.Values{"tag": {"kubedojo"}, "type": {"dash-db"}}, &found)
	if err == nil && len(found) < want {
		err = fmt.Errorf("%d of %d dashboards are loaded", len(found), want)
	}
	add(fmt.Sprintf("Grafana has the %d kubedojo dashboards", want), err)

	rules, rerr := provisioning.Rules(provisioning.RulesSource)
	local := 0
	for _, r := range rules {
		for _, sc := range r.Scope {
			if sc == provisioning.ScopeLocal {
				local++
			}
		}
	}
	var alerting struct {
		Data struct {
			Groups []struct {
				Rules []struct {
					Name      string `json:"name"`
					Health    string `json:"health"`
					LastError string `json:"lastError"`
				} `json:"rules"`
			} `json:"groups"`
		} `json:"data"`
	}
	err = rerr
	if err == nil {
		err = s.eventually(ctx, wait, func() error {
			if err := s.grafana(ctx, "/api/prometheus/grafana/api/v1/rules", nil, &alerting); err != nil {
				return err
			}
			n := 0
			for _, g := range alerting.Data.Groups {
				for _, r := range g.Rules {
					if !strings.HasPrefix(r.Name, "Kubedojo") {
						continue
					}
					n++
					// "nodata" is a healthy rule whose condition does not hold.
					if r.Health == "error" {
						return fmt.Errorf("the rule %s cannot be evaluated: %s", r.Name, r.LastError)
					}
				}
			}
			if n < local {
				return fmt.Errorf("%d of %d alert rules are loaded", n, local)
			}
			return nil
		})
	}
	add(fmt.Sprintf("Grafana evaluates the %d alert rules", local), err)

	// One span, one log record and one metric sample, written and read back.
	traceID, err := telemetry.Canary(ctx, s.Endpoint())
	add("the OTLP receiver takes a trace, a log record and a metric", err)
	if err != nil {
		return checks
	}
	proxy := "/api/datasources/proxy/uid/"
	now := time.Now()
	window := url.Values{"start": {fmt.Sprint(now.Add(-10 * time.Minute).Unix())}, "end": {fmt.Sprint(now.Add(time.Minute).Unix())}}

	add("Tempo returns the trace", s.eventually(ctx, wait, func() error {
		var tr struct {
			Batches []json.RawMessage `json:"batches"`
		}
		if err := s.grafana(ctx, proxy+"tempo/api/traces/"+traceID, nil, &tr); err != nil {
			return err
		}
		if len(tr.Batches) == 0 {
			return fmt.Errorf("trace %s is not there yet", traceID)
		}
		return nil
	}))

	// The query of Grafana's link from a trace to its logs.
	add("Loki returns the log record by its trace ID (trace → logs)", s.eventually(ctx, wait, func() error {
		var res struct {
			Data struct {
				Result []json.RawMessage `json:"result"`
			} `json:"data"`
		}
		q := url.Values{"query": {fmt.Sprintf(`{service_name=%q} | trace_id = %q`, telemetry.CanaryService, traceID)}}
		for k, v := range window {
			q[k] = v
		}
		if err := s.grafana(ctx, proxy+"loki/loki/api/v1/query_range", q, &res); err != nil {
			return err
		}
		if len(res.Data.Result) == 0 {
			return fmt.Errorf("no log line with trace_id %s yet", traceID)
		}
		return nil
	}))

	add("Prometheus returns the metric", s.eventually(ctx, wait, func() error {
		var res struct {
			Data struct {
				Result []json.RawMessage `json:"result"`
			} `json:"data"`
		}
		q := url.Values{"query": {fmt.Sprintf(`%s{service_name=%q}`, telemetry.CanaryMetric, telemetry.CanaryService)}}
		if err := s.grafana(ctx, proxy+"prometheus/api/v1/query", q, &res); err != nil {
			return err
		}
		if len(res.Data.Result) == 0 {
			return fmt.Errorf("no sample of %s yet", telemetry.CanaryMetric)
		}
		return nil
	}))

	add("the metric sample carries the trace ID as an exemplar (metric → trace)", s.eventually(ctx, wait, func() error {
		var res struct {
			Data []struct {
				Exemplars []struct {
					Labels map[string]string `json:"labels"`
				} `json:"exemplars"`
			} `json:"data"`
		}
		q := url.Values{"query": {telemetry.CanaryMetric}}
		for k, v := range window {
			q[k] = v
		}
		if err := s.grafana(ctx, proxy+"prometheus/api/v1/query_exemplars", q, &res); err != nil {
			return err
		}
		for _, series := range res.Data {
			for _, e := range series.Exemplars {
				if e.Labels["trace_id"] == traceID {
					return nil
				}
			}
		}
		return fmt.Errorf("no exemplar with trace_id %s yet", traceID)
	}))
	return checks
}

// Failed returns the checks that did not pass.
func Failed(checks []Check) []Check {
	var out []Check
	for _, c := range checks {
		if c.Err != nil {
			out = append(out, c)
		}
	}
	return out
}

// eventually runs f until it succeeds or wait is over, and returns its last error.
func (s *Stack) eventually(ctx context.Context, wait time.Duration, f func() error) error {
	deadline := time.Now().Add(wait)
	for {
		err := f()
		if err == nil || time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(s.poll):
		}
	}
}

// grafana GETs a path of Grafana's API and decodes the JSON answer.
func (s *Stack) grafana(ctx context.Context, path string, query url.Values, into any) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	u := s.GrafanaURL() + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d: %s", path, res.StatusCode, strings.TrimSpace(string(body[:min(len(body), 200)])))
	}
	return json.Unmarshal(body, into)
}
