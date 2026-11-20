package main

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/Mistral-valaise/kubedojo/content"
	"github.com/Mistral-valaise/kubedojo/internal/catalog"
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

type correlation struct {
	spans  *tracetest.SpanRecorder
	tracer trace.Tracer
	reader *sdkmetric.ManualReader
	logs   *logRecorder
	mgr    *session.Manager
	cat    *catalog.Catalog
}

// newCorrelation wires a session manager to telemetry exactly as the server
// does (telemetryHooks), with in-memory exporters instead of OTLP.
func newCorrelation(t *testing.T) *correlation {
	t.Helper()
	c := &correlation{spans: tracetest.NewSpanRecorder(), reader: sdkmetric.NewManualReader(), logs: &logRecorder{}}
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(c.spans))
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(c.reader))
	c.tracer = tp.Tracer("test")
	tel := telemetry.New(tp, mp, slog.New(c.logs))

	var err error
	if c.cat, err = catalog.Load(content.FS); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	c.mgr, err = session.NewManager(session.Config{Catalog: c.cat, Store: st, Driver: &sandboxtest.Driver{},
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
	// The spans end just after the state is written.
	for deadline := time.Now().Add(5 * time.Second); len(c.spans.Ended()) < 7 && time.Now().Before(deadline); {
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
