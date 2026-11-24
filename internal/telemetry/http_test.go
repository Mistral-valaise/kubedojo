package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/Mistral-valaise/kubedojo/internal/span"
)

// A handler behind HTTP reaches the request span through the context alone:
// span.Current is the span of the request, span.Start a child of it.
func TestHTTPPutsTheTracerIntoTheRequestContext(t *testing.T) {
	tel, rec := recordingTelemetry()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/sessions/{id}/start", func(w http.ResponseWriter, r *http.Request) {
		if span.From(r.Context()) != span.Tracer(tel) {
			t.Error("the request context does not carry the tracer")
		}
		span.Current(r.Context()).Set(span.String("kubedojo.session.id", r.PathValue("id")))
		span.Current(r.Context()).Event("exam.started")
		_, child := span.Start(r.Context(), "work")
		child.End(nil)
		w.WriteHeader(204)
	})
	res := httptest.NewRecorder()
	tel.HTTP(mux).ServeHTTP(res, httptest.NewRequest("POST", "/api/sessions/abc/start", nil))
	if res.Code != 204 {
		t.Fatalf("status %d", res.Code)
	}

	// The span is named after the route, which the mux only knows once it
	// has matched: the wrapper in between must hand it back to otelhttp.
	request := ended(t, rec, "POST /api/sessions/{id}/start")
	if request.SpanKind() != trace.SpanKindServer {
		t.Errorf("kind %v, want server", request.SpanKind())
	}
	a := attrs(request.Attributes())
	if a["kubedojo.session.id"].AsString() != "abc" {
		t.Errorf("the attribute set through span.Current is missing: %v", request.Attributes())
	}
	if a["http.route"].AsString() != "/api/sessions/{id}/start" {
		t.Errorf("http.route = %q", a["http.route"].AsString())
	}
	if ev := request.Events(); len(ev) != 1 || ev[0].Name != "exam.started" {
		t.Errorf("events of the request span: %v", ev)
	}
	if child := ended(t, rec, "work"); child.Parent().SpanID() != request.SpanContext().SpanID() {
		t.Errorf("the span started in the handler is not a child of the request span")
	}

	// The response names the trace, in the traceparent format.
	header := res.Header().Get("Server-Timing")
	m := regexp.MustCompile(`^traceparent;desc="00-([0-9a-f]{32})-([0-9a-f]{16})-01"$`).FindStringSubmatch(header)
	if m == nil {
		t.Fatalf("Server-Timing = %q", header)
	}
	if m[1] != request.SpanContext().TraceID().String() || m[2] != request.SpanContext().SpanID().String() {
		t.Errorf("Server-Timing names %s/%s, the request span is %s/%s", m[1], m[2], request.SpanContext().TraceID(), request.SpanContext().SpanID())
	}
}

// What is not traced gets no span and no header, but its handler can still
// call span.Current and span.Start: they do nothing harmful.
func TestHTTPLeavesUntracedRequestsAlone(t *testing.T) {
	tel, rec := recordingTelemetry()
	h := tel.HTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span.Current(r.Context()).Set(span.String("k", "v"))
		span.Current(r.Context()).Event("e")
		w.WriteHeader(204)
	}))
	for _, c := range []struct {
		path    string
		upgrade bool
	}{{"/api/health", false}, {"/api/ready", false}, {"/assets/app.js", false}, {"/api/sessions/abc/terminal", true}} {
		req := httptest.NewRequest("GET", c.path, nil)
		if c.upgrade {
			req.Header.Set("Upgrade", "websocket")
		}
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if got := res.Header().Get("Server-Timing"); got != "" {
			t.Errorf("%s: Server-Timing = %q, want none", c.path, got)
		}
	}
	if n := len(rec.Ended()); n != 0 {
		t.Errorf("%d spans for requests that are not traced", n)
	}
}

// The route reaches the request metric as well: the dashboards group by it.
func TestHTTPRecordsTheRouteOnTheRequestMetric(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(tracetest.NewSpanRecorder()))
	tel := New(tp, sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)), slog.New(slog.NewTextHandler(io.Discard, nil)))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/tracks/{track}", func(w http.ResponseWriter, r *http.Request) {})
	tel.HTTP(mux).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/tracks/cka", nil))

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "http.server.request.duration" {
				continue
			}
			for _, p := range m.Data.(metricdata.Histogram[float64]).DataPoints {
				if v, _ := p.Attributes.Value(attribute.Key("http.route")); v.AsString() != "/api/tracks/{track}" {
					t.Errorf("http.route of the request metric = %q", v.AsString())
				}
				return
			}
		}
	}
	t.Fatal("http.server.request.duration was not recorded")
}

// Work that outlives its request keeps the trace of the request and the
// tracer, and no longer ends with the request.
func TestDetachKeepsTheTraceAndTheTracer(t *testing.T) {
	tel, rec := recordingTelemetry()
	reqCtx, cancel := context.WithCancel(context.Background())
	reqCtx, request := tel.Start(reqCtx, "request")
	base := context.Background()

	bg := Detach(base, reqCtx)
	request.End(nil)
	cancel()
	if bg.Err() != nil {
		t.Fatal("the detached context ended with the request")
	}
	if span.From(bg) != span.Tracer(tel) {
		t.Fatal("the detached context lost the tracer")
	}
	_, work := span.Start(bg, "work")
	work.End(nil)
	w, r := ended(t, rec, "work"), ended(t, rec, "request")
	if w.SpanContext().TraceID() != r.SpanContext().TraceID() || w.Parent().SpanID() != r.SpanContext().SpanID() {
		t.Errorf("work is not a child of the request: trace %s parent %s", w.SpanContext().TraceID(), w.Parent().SpanID())
	}

	// A request without a tracer detaches to a context without one.
	_, s := span.Start(Detach(base, context.Background()), "nothing")
	s.End(nil)
	if len(rec.Ended()) != 2 {
		t.Errorf("a span was recorded without a tracer in the context")
	}
}

// Lines on stderr name the trace and the span they were written in, in serve
// (JSON) as in up (text). Outside a span nothing is added.
func TestStderrLinesCarryTheTrace(t *testing.T) {
	var buf bytes.Buffer
	tel, _ := recordingTelemetry()
	tel = New(tel.tp, tel.mp, slog.New(slog.NewJSONHandler(&buf, nil)))
	ctx, s := tel.Start(context.Background(), "work")
	tel.Logger.With("component", "test").InfoContext(ctx, "inside", "session", "abc")
	tel.Logger.Info("outside")
	s.End(nil)

	var lines []map[string]any
	for _, raw := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		var l map[string]any
		if err := json.Unmarshal(raw, &l); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		lines = append(lines, l)
	}
	sc := trace.SpanContextFromContext(ctx)
	if len(lines) != 2 || lines[0]["trace_id"] != sc.TraceID().String() || lines[0]["span_id"] != sc.SpanID().String() ||
		lines[0]["session"] != "abc" || lines[0]["component"] != "test" {
		t.Errorf("the line written inside the span: %v", lines)
	}
	if _, ok := lines[1]["trace_id"]; ok {
		t.Errorf("the line written outside a span names a trace: %v", lines[1])
	}
}

// Without an endpoint telemetry is off. The logger still works, spans record
// nothing, and no line gets a trace.
func TestSetupWithoutAnEndpoint(t *testing.T) {
	for _, k := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT"} {
		t.Setenv(k, "")
	}
	var buf bytes.Buffer
	tel, err := Setup(context.Background(), "test", slog.New(slog.NewJSONHandler(&buf, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if tel.Enabled {
		t.Fatal("telemetry is enabled without an endpoint")
	}
	ctx, s := tel.Start(context.Background(), "work")
	tel.Logger.InfoContext(ctx, "line")
	s.End(nil)
	tel.Rejected("host")
	tel.StoreOperation("ping", 0)
	tel.TerminalClosed(ctx, 0)
	var l map[string]any
	if err := json.Unmarshal(buf.Bytes(), &l); err != nil {
		t.Fatalf("%s: %v", buf.Bytes(), err)
	}
	if _, ok := l["trace_id"]; ok || l["msg"] != "line" {
		t.Errorf("line without telemetry: %v", l)
	}

	// A span of somebody else's SDK in the context is named all the same:
	// kubedojo embedded into a process that traces.
	tp := sdktrace.NewTracerProvider()
	ctx, other := tp.Tracer("other").Start(context.Background(), "other")
	buf.Reset()
	tel.Logger.InfoContext(ctx, "line")
	other.End()
	if err := json.Unmarshal(buf.Bytes(), &l); err != nil {
		t.Fatal(err)
	}
	if l["trace_id"] != other.SpanContext().TraceID().String() {
		t.Errorf("trace_id = %v, want %s", l["trace_id"], other.SpanContext().TraceID())
	}
}
