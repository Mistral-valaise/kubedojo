package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/Mistral-valaise/kubedojo/content"
	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/checker"
	"github.com/Mistral-valaise/kubedojo/internal/dockerhost"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox/sandboxtest"
	"github.com/Mistral-valaise/kubedojo/internal/session"
	"github.com/Mistral-valaise/kubedojo/internal/store"
	"github.com/Mistral-valaise/kubedojo/internal/telemetry"
)

// logLine is a log record together with the span it was written in.
type logLine struct {
	msg   string
	span  trace.SpanContext
	attrs map[string]string
}

// logRecorder is a slog handler that keeps what the OpenTelemetry log bridge
// reads from a record: the message, the attributes and the span of the context.
type logRecorder struct {
	mu    sync.Mutex
	lines []logLine
}

func (r *logRecorder) Enabled(context.Context, slog.Level) bool { return true }
func (r *logRecorder) WithAttrs([]slog.Attr) slog.Handler       { return r }
func (r *logRecorder) WithGroup(string) slog.Handler            { return r }
func (r *logRecorder) Handle(ctx context.Context, rec slog.Record) error {
	l := logLine{msg: rec.Message, span: trace.SpanContextFromContext(ctx), attrs: map[string]string{}}
	rec.Attrs(func(a slog.Attr) bool { l.attrs[a.Key] = a.Value.String(); return true })
	r.mu.Lock()
	r.lines = append(r.lines, l)
	r.mu.Unlock()
	return nil
}

func (r *logRecorder) all() []logLine {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]logLine(nil), r.lines...)
}

// stderrRecorder is where the server's own log lines go: what `kubedojo
// serve` writes to stderr as JSON.
type stderrRecorder struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *stderrRecorder) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *stderrRecorder) lines(t *testing.T) []map[string]any {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []map[string]any
	for _, raw := range bytes.Split(bytes.TrimSpace(s.buf.Bytes()), []byte("\n")) {
		if len(raw) == 0 {
			continue
		}
		var l map[string]any
		if err := json.Unmarshal(raw, &l); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		out = append(out, l)
	}
	return out
}

// both sends every record to two handlers.
type both [2]slog.Handler

func (b both) Enabled(context.Context, slog.Level) bool { return true }
func (b both) Handle(ctx context.Context, r slog.Record) error {
	return errors.Join(b[0].Handle(ctx, r.Clone()), b[1].Handle(ctx, r.Clone()))
}
func (b both) WithAttrs(a []slog.Attr) slog.Handler {
	return both{b[0].WithAttrs(a), b[1].WithAttrs(a)}
}
func (b both) WithGroup(n string) slog.Handler { return both{b[0].WithGroup(n), b[1].WithGroup(n)} }

// breakable is the fake driver with a sandbox that can stop taking commands.
type breakable struct {
	sandboxtest.Driver
	mu     sync.Mutex
	broken error
}

func (d *breakable) Exec(ctx context.Context, h dockerhost.Host, sid string, t sandbox.Target, user, script string, env map[string]string) (sandbox.ExecResult, error) {
	d.mu.Lock()
	err := d.broken
	d.mu.Unlock()
	if err != nil {
		return sandbox.ExecResult{}, err
	}
	return d.Driver.Exec(ctx, h, sid, t, user, script, env)
}

func (d *breakable) breakWith(err error) {
	d.mu.Lock()
	d.broken = err
	d.mu.Unlock()
}

type correlation struct {
	spans  *tracetest.SpanRecorder
	tracer trace.Tracer
	reader *sdkmetric.ManualReader
	logs   *logRecorder
	stderr *stderrRecorder
	drv    *breakable
	tel    *telemetry.Telemetry
	st     *store.Store
	mgr    *session.Manager
	cat    *catalog.Catalog
}

// newCorrelation wires a session manager to telemetry exactly as the server
// does (telemetryHooks), with in-memory exporters instead of OTLP.
func newCorrelation(t *testing.T) *correlation {
	t.Helper()
	c := &correlation{spans: tracetest.NewSpanRecorder(), reader: sdkmetric.NewManualReader(), logs: &logRecorder{},
		stderr: &stderrRecorder{}, drv: &breakable{}}
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(c.spans))
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(c.reader))
	c.tracer = tp.Tracer("test")
	tel := telemetry.New(tp, mp, slog.New(both{c.logs, slog.NewJSONHandler(c.stderr, nil)}))
	c.tel = tel

	var err error
	if c.cat, err = catalog.Load(content.FS); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	c.st = st
	c.mgr, err = session.NewManager(session.Config{Catalog: c.cat, Store: st, Driver: c.drv,
		Logger: tel.Logger, Hooks: telemetryHooks(tel), GradeSettle: -1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.mgr.Close)
	return c
}

// span returns the ended span with the name, failing the test if there is
// none or more than one.
func (c *correlation) span(t *testing.T, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	var found []sdktrace.ReadOnlySpan
	for _, s := range c.spans.Ended() {
		if s.Name() == name {
			found = append(found, s)
		}
	}
	if len(found) != 1 {
		var names []string
		for _, s := range c.spans.Ended() {
			names = append(names, s.Name())
		}
		t.Fatalf("%d spans named %q; ended spans: %v", len(found), name, names)
	}
	return found[0]
}

func attr(s sdktrace.ReadOnlySpan, key string) string {
	for _, a := range s.Attributes() {
		if string(a.Key) == key {
			return a.Value.AsString()
		}
	}
	return ""
}

// exemplars returns the trace IDs attached to the samples of a metric, and
// the attribute keys of its data points.
func (c *correlation) exemplars(t *testing.T, name string) (traces []trace.TraceID, keys map[string]bool) {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := c.reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	keys = map[string]bool{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			switch d := m.Data.(type) {
			case metricdata.Histogram[float64]:
				for _, p := range d.DataPoints {
					for _, kv := range p.Attributes.ToSlice() {
						keys[string(kv.Key)] = true
					}
					for _, e := range p.Exemplars {
						traces = append(traces, trace.TraceID(e.TraceID))
					}
				}
			case metricdata.Sum[int64]:
				for _, p := range d.DataPoints {
					for _, kv := range p.Attributes.ToSlice() {
						keys[string(kv.Key)] = true
					}
					for _, e := range p.Exemplars {
						traces = append(traces, trace.TraceID(e.TraceID))
					}
				}
			default:
				t.Fatalf("metric %s has unexpected data %T", name, m.Data)
			}
		}
	}
	return traces, keys
}

func (c *correlation) ready(t *testing.T, sid string) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		v, err := c.mgr.View(sid)
		if err != nil {
			t.Fatal(err)
		}
		if len(v.Questions) > 0 && v.Questions[0].Status == "ready" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the question never became ready: %+v", v.Questions)
		}
	}
}

// One request to prepare a question: the cluster is created after the request
// was answered. Its spans, its log lines and its metric sample must all belong
// to the trace of that request, or Grafana has nothing to link.
func TestBackgroundWorkBelongsToTheTraceOfItsRequest(t *testing.T) {
	c := newCorrelation(t)
	ses, err := c.mgr.Practice("cka", "")
	if err != nil {
		t.Fatal(err)
	}
	q := c.cat.Tracks["cka"].Questions[0]

	// The request: its context ends as soon as it is answered.
	reqCtx, cancel := context.WithCancel(context.Background())
	reqCtx, request := c.tracer.Start(reqCtx, "POST /api/sessions/{id}/questions/{q}/prepare")
	if err := c.mgr.PrepareContext(reqCtx, ses.ID, q.ID, false); err != nil {
		t.Fatal(err)
	}
	request.End()
	cancel()
	c.ready(t, ses.ID)
	// The spans end just after the state is written; question.prepare is the last.
	for deadline := time.Now().Add(5 * time.Second); len(c.named("question.prepare")) == 0 && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}
	traceID := request.SpanContext().TraceID()

	// Spans: one tree below the request.
	prepare := c.span(t, "question.prepare")
	provision := c.span(t, "sandbox.provision")
	for name, parent := range map[string]trace.SpanID{
		"question.prepare":         request.SpanContext().SpanID(),
		"sandbox.provision":        prepare.SpanContext().SpanID(),
		"sandbox.host.acquire":     provision.SpanContext().SpanID(),
		"sandbox.env.create":       provision.SpanContext().SpanID(),
		"sandbox.terminal.connect": provision.SpanContext().SpanID(),
		"question.setup":           prepare.SpanContext().SpanID(),
	} {
		s := c.span(t, name)
		if s.SpanContext().TraceID() != traceID {
			t.Errorf("span %s is in trace %s, the request in %s", name, s.SpanContext().TraceID(), traceID)
		}
		if s.Parent().SpanID() != parent {
			t.Errorf("span %s has parent %s, want %s", name, s.Parent().SpanID(), parent)
		}
		if attr(s, "kubedojo.session.id") != ses.ID || attr(s, "kubedojo.track") != "cka" {
			t.Errorf("span %s lacks the session or the track: %v", name, s.Attributes())
		}
	}
	if attr(prepare, "kubedojo.question") != q.ID {
		t.Errorf("question.prepare names the question %q, want %q", attr(prepare, "kubedojo.question"), q.ID)
	}

	// Logs: the progress lines carry the trace and the span they were written in.
	var inTrace int
	for _, l := range c.logs.all() {
		if !strings.Contains(l.msg, "is ready") {
			continue
		}
		if l.span.TraceID() != traceID {
			t.Errorf("log line %q is in trace %s, want %s", l.msg, l.span.TraceID(), traceID)
		}
		if l.attrs["session"] != ses.ID {
			t.Errorf("log line %q lacks the session: %v", l.msg, l.attrs)
		}
		inTrace++
	}
	if inTrace == 0 {
		t.Fatalf("no progress line was logged: %+v", c.logs.all())
	}

	// Metrics: the sample of the provisioning carries the trace as an
	// exemplar, and no data point is labelled with a session.
	traces, keys := c.exemplars(t, "kubedojo.sandbox.provision.duration")
	if len(traces) != 1 || traces[0] != traceID {
		t.Errorf("exemplars of the provisioning duration: %v, want the trace %s", traces, traceID)
	}
	for k := range keys {
		if strings.Contains(k, "session") || strings.Contains(k, "user") || strings.Contains(k, "owner") {
			t.Errorf("the metric is labelled with %q; sessions and users must not become labels", k)
		}
	}
}

// Checking a question happens inside the request.
func TestCheckIsASpanOfItsRequestWithAnExemplar(t *testing.T) {
	c := newCorrelation(t)
	ses, err := c.mgr.Practice("cka", "")
	if err != nil {
		t.Fatal(err)
	}
	q := c.cat.Tracks["cka"].Questions[0]
	if err := c.mgr.Prepare(ses.ID, q.ID, false); err != nil {
		t.Fatal(err)
	}
	c.ready(t, ses.ID)

	ctx, request := c.tracer.Start(context.Background(), "POST /api/sessions/{id}/questions/{q}/check")
	if _, err := c.mgr.Check(ctx, ses.ID, q.ID); err != nil {
		t.Fatal(err)
	}
	request.End()

	check := c.span(t, "question.check")
	if check.SpanContext().TraceID() != request.SpanContext().TraceID() || check.Parent().SpanID() != request.SpanContext().SpanID() {
		t.Errorf("question.check is not a child of the request: trace %s parent %s", check.SpanContext().TraceID(), check.Parent().SpanID())
	}
	if attr(check, "kubedojo.question") != q.ID {
		t.Errorf("question.check names the question %q", attr(check, "kubedojo.question"))
	}
	traces, keys := c.exemplars(t, "kubedojo.check.runs")
	if len(traces) != 1 || traces[0] != request.SpanContext().TraceID() {
		t.Errorf("exemplars of the check counter: %v, want the trace %s", traces, request.SpanContext().TraceID())
	}
	if !keys["question"] || keys["session"] {
		t.Errorf("labels of the check counter: %v", keys)
	}
}

// Work the clock starts has no request. It still gets a trace of its own, so
// that its log lines can be found together.
func TestWorkWithoutARequestStartsItsOwnTrace(t *testing.T) {
	c := newCorrelation(t)
	ses, err := c.mgr.Practice("cka", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.mgr.Prepare(ses.ID, c.cat.Tracks["cka"].Questions[0].ID, false); err != nil {
		t.Fatal(err)
	}
	c.ready(t, ses.ID)
	if err := c.mgr.Destroy(context.Background(), ses.ID); err != nil {
		t.Fatal(err)
	}
	destroy := c.span(t, "sandbox.destroy")
	if !destroy.SpanContext().TraceID().IsValid() || destroy.Parent().IsValid() {
		t.Errorf("sandbox.destroy without a request: trace %s, parent %s; want a root span", destroy.SpanContext().TraceID(), destroy.Parent().SpanID())
	}
}

// named returns the ended spans with the name.
func (c *correlation) named(name string) []sdktrace.ReadOnlySpan {
	var out []sdktrace.ReadOnlySpan
	for _, s := range c.spans.Ended() {
		if s.Name() == name {
			out = append(out, s)
		}
	}
	return out
}

// children returns the ended spans directly below a span.
func (c *correlation) children(parent sdktrace.ReadOnlySpan) []sdktrace.ReadOnlySpan {
	var out []sdktrace.ReadOnlySpan
	for _, s := range c.spans.Ended() {
		if s.Parent().SpanID() == parent.SpanContext().SpanID() && s.SpanContext().TraceID() == parent.SpanContext().TraceID() {
			out = append(out, s)
		}
	}
	return out
}

// call sends one request through the handler the server serves: telemetry in
// front of the API. It returns the response and the span of the request.
func (c *correlation) call(t *testing.T, h http.Handler, method, path, body string) (*httptest.ResponseRecorder, sdktrace.ReadOnlySpan) {
	t.Helper()
	before := len(c.spans.Ended())
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	// The request span ends when the handler returns; spans of work that
	// goes on in the background may end before or after it.
	for _, s := range c.spans.Ended()[before:] {
		if s.SpanKind() == trace.SpanKindServer {
			return res, s
		}
	}
	t.Fatalf("%s %s: no request span ended; status %d", method, path, res.Code)
	return nil, nil
}

// One learner, followed through the API as the server serves it: every
// request is a trace that reads from top to bottom, the request names its
// session and question, a wrong answer is not an error, and a sandbox that
// does not answer is.
func TestTheTraceOfARequestReadsAsAStory(t *testing.T) {
	c := newCorrelation(t)
	parts := serverParts{catalog: c.cat, sessions: c.mgr, store: c.st, telemetry: c.tel, log: c.tel.Logger, ai: &aiFlags{}}
	h := c.tel.HTTP(newAPIServer(parts).Handler())
	q := c.cat.Tracks["cka"].Questions[0]

	// Opening a practice session: one span below the request, and the
	// response names the trace.
	res, request := c.call(t, h, "POST", "/api/sessions", `{"mode":"practice","track":"cka"}`)
	if res.Code != 201 {
		t.Fatalf("create session: %d %s", res.Code, res.Body)
	}
	var ses struct{ ID string }
	if err := json.Unmarshal(res.Body.Bytes(), &ses); err != nil || ses.ID == "" {
		t.Fatalf("create session: %s", res.Body)
	}
	if request.Name() != "POST /api/sessions" {
		t.Errorf("the request span is named %q", request.Name())
	}
	open := c.span(t, "session.practice.open")
	if open.Parent().SpanID() != request.SpanContext().SpanID() || attr(open, "kubedojo.session.id") != ses.ID {
		t.Errorf("session.practice.open: parent %s, %v", open.Parent().SpanID(), open.Attributes())
	}
	if want := `traceparent;desc="00-` + request.SpanContext().TraceID().String() + "-" + request.SpanContext().SpanID().String() + `-01"`; res.Header().Get("Server-Timing") != want {
		t.Errorf("Server-Timing = %q, want %q", res.Header().Get("Server-Timing"), want)
	}

	base := "/api/sessions/" + ses.ID + "/questions/" + q.ID
	res, prepare := c.call(t, h, "POST", base+"/prepare", "")
	if res.Code != 202 {
		t.Fatalf("prepare: %d %s", res.Code, res.Body)
	}
	c.ready(t, ses.ID)
	for deadline := time.Now().Add(5 * time.Second); len(c.named("question.prepare")) == 0 && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}
	// The request names its session and its question, also when the work is
	// done by a span below it (or by none at all).
	for key, want := range map[string]string{"kubedojo.session.id": ses.ID, "kubedojo.track": "cka", "kubedojo.mode": "practice", "kubedojo.question": q.ID} {
		if attr(prepare, key) != want {
			t.Errorf("the request span of prepare has %s = %q, want %q", key, attr(prepare, key), want)
		}
	}
	// A command outside a check says what it is for.
	if exec := c.span(t, "sandbox.exec"); attr(exec, "kubedojo.exec.purpose") != session.ExecSetup || exec.Parent().SpanID() != c.span(t, "question.setup").SpanContext().SpanID() {
		t.Errorf("sandbox.exec: %v", exec.Attributes())
	}

	// stderr: the progress lines of the preparation name its trace.
	found := false
	for _, l := range c.stderr.lines(t) {
		if msg, _ := l["msg"].(string); strings.Contains(msg, "is ready") {
			found = true
			if l["trace_id"] != prepare.SpanContext().TraceID().String() || l["span_id"] == nil || l["session"] != ses.ID {
				t.Errorf("stderr line %q: trace_id %v span_id %v session %v, want the trace %s", msg, l["trace_id"], l["span_id"], l["session"], prepare.SpanContext().TraceID())
			}
		}
	}
	if !found {
		t.Errorf("no progress line on stderr: %v", c.stderr.lines(t))
	}

	// Grading. The fake sandbox prints nothing, so every check of this
	// question fails: a learner who has not solved it yet.
	res, check := c.call(t, h, "POST", base+"/check", "")
	if res.Code != 200 {
		t.Fatalf("check: %d %s", res.Code, res.Body)
	}
	question := c.span(t, "question.check")
	if question.Parent().SpanID() != check.SpanContext().SpanID() {
		t.Error("question.check is not a child of the request span")
	}
	checks := c.children(question)
	if len(checks) != len(q.Checks) || len(checks) == 0 {
		t.Fatalf("%d spans below question.check for %d checks", len(checks), len(q.Checks))
	}
	for _, s := range checks {
		if s.Name() != "check.run" || s.SpanContext().TraceID() != check.SpanContext().TraceID() {
			t.Errorf("below question.check: %s in trace %s", s.Name(), s.SpanContext().TraceID())
		}
		if s.Status().Code != codes.Unset || attr(s, "kubedojo.check.outcome") != checker.OutcomeFailed {
			t.Errorf("a failed check %s: status %v, outcome %q; want Unset and failed", attr(s, "kubedojo.check.id"), s.Status().Code, attr(s, "kubedojo.check.outcome"))
		}
		if len(c.children(s)) != 0 {
			t.Errorf("check.run %s has spans below it", attr(s, "kubedojo.check.id"))
		}
	}
	if question.Status().Code != codes.Unset || check.Status().Code != codes.Unset {
		t.Errorf("a question that is not solved: question.check %v, request %v; want Unset", question.Status().Code, check.Status().Code)
	}

	// Hints and the solution are small: no span, an event on the request.
	res, hint := c.call(t, h, "GET", base+"/hints/1", "")
	if res.Code != 200 || len(c.children(hint)) != 0 {
		t.Fatalf("hint: %d, %d spans below the request", res.Code, len(c.children(hint)))
	}
	if ev := hint.Events(); len(ev) != 1 || ev[0].Name != "hint.revealed" || ev[0].Attributes[0].Value.AsInt64() != 1 {
		t.Errorf("events of the hint request: %v", ev)
	}
	if attr(hint, "kubedojo.question") != q.ID || attr(hint, "kubedojo.session.id") != ses.ID {
		t.Errorf("the hint request does not name its session and question: %v", hint.Attributes())
	}
	_, solution := c.call(t, h, "GET", base+"/solution", "")
	if ev := solution.Events(); len(ev) != 1 || ev[0].Name != "solution.revealed" {
		t.Errorf("events of the solution request: %v", ev)
	}

	// The sandbox stops answering between the request and its checks. Now
	// the checks could not be evaluated, and that is an error.
	c.drv.breakWith(errors.New("cannot connect to the Docker daemon"))
	res, broken := c.call(t, h, "POST", base+"/check", "")
	if res.Code != 200 {
		t.Fatalf("check with a broken sandbox: %d %s", res.Code, res.Body)
	}
	var graded sdktrace.ReadOnlySpan
	for _, s := range c.named("question.check") {
		if s.Parent().SpanID() == broken.SpanContext().SpanID() {
			graded = s
		}
	}
	if graded == nil {
		t.Fatal("no question.check below the second check request")
	}
	if graded.Status().Code != codes.Error || attr(graded, "error.type") != checker.OutcomeGraderError {
		t.Errorf("question.check with a broken sandbox: status %v, error.type %q", graded.Status().Code, attr(graded, "error.type"))
	}
	for _, s := range c.children(graded) {
		if s.Status().Code != codes.Error || attr(s, "kubedojo.check.outcome") != checker.OutcomeGraderError || attr(s, "error.type") != checker.OutcomeGraderError {
			t.Errorf("check.run %s with a broken sandbox: status %v, outcome %q", attr(s, "kubedojo.check.id"), s.Status().Code, attr(s, "kubedojo.check.outcome"))
		}
	}
}

// The store is observed through the hooks of the session manager, and its
// operations become a histogram that is labelled with the operation only.
func TestStoreOperationsBecomeAMetric(t *testing.T) {
	c := newCorrelation(t)
	if _, err := c.mgr.Practice("cka", ""); err != nil {
		t.Fatal(err)
	}
	var rm metricdata.ResourceMetrics
	if err := c.reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	ops := map[string]bool{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "kubedojo.store.operation.duration" {
				continue
			}
			for _, p := range m.Data.(metricdata.Histogram[float64]).DataPoints {
				if p.Attributes.Len() != 1 {
					t.Errorf("labels of the store histogram: %v", p.Attributes.ToSlice())
				}
				v, _ := p.Attributes.Value("operation")
				ops[v.AsString()] = true
			}
		}
	}
	if !ops["save_session"] || !ops["load_sessions"] {
		t.Errorf("operations recorded: %v", ops)
	}
}

// A terminal is traced at its two ends. The sample of its duration, recorded
// when it ends, carries the trace of the attach as an exemplar, and so does
// the line that says it ended.
func TestTheEndOfATerminalBelongsToTheTraceOfItsAttach(t *testing.T) {
	c := newCorrelation(t)
	ses, err := c.mgr.Practice("cka", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.mgr.Prepare(ses.ID, c.cat.Tracks["cka"].Questions[0].ID, false); err != nil {
		t.Fatal(err)
	}
	c.ready(t, ses.ID)
	// The WebSocket request is not traced: the attach starts its own trace.
	tty, err := c.mgr.Attach(context.Background(), ses.ID, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	attach := c.span(t, "terminal.attach")
	if attach.Parent().IsValid() {
		t.Errorf("terminal.attach has the parent %s, want a root span", attach.Parent().SpanID())
	}
	if err := session.CloseTerminal(tty, session.TerminalShellExited); err != nil {
		t.Fatal(err)
	}
	traceID := attach.SpanContext().TraceID()
	traces, keys := c.exemplars(t, "kubedojo.terminal.session.duration")
	if len(traces) != 1 || traces[0] != traceID || len(keys) != 0 {
		t.Errorf("the duration of the terminal: exemplars %v, labels %v; want the trace %s and no labels", traces, keys, traceID)
	}
	found := false
	for _, l := range c.stderr.lines(t) {
		if l["msg"] == "terminal closed" {
			found = true
			if l["trace_id"] != traceID.String() || l["reason"] != session.TerminalShellExited || l["session"] != ses.ID {
				t.Errorf("the line of the terminal's end: %v", l)
			}
		}
	}
	if !found {
		t.Error("no line says the terminal closed")
	}
}
