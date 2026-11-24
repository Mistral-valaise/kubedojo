// Package telemetry wires kubedojo into an OpenTelemetry pipeline: traces,
// metrics and logs are exported over OTLP/HTTP when an endpoint is configured
// (OTEL_EXPORTER_OTLP_ENDPOINT), e.g. to Grafana Alloy in front of
// Tempo, Mimir and Loki. Without an endpoint everything is a no-op.
package telemetry

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/Mistral-valaise/kubedojo/internal/span"
)

const scope = "github.com/Mistral-valaise/kubedojo"

// Telemetry holds the instruments kubedojo records to.
type Telemetry struct {
	Enabled  bool
	Logger   *slog.Logger
	shutdown []func(context.Context) error

	// The providers the instruments come from. The HTTP instrumentation
	// records to the same ones.
	tp trace.TracerProvider
	mp metric.MeterProvider

	tracer    trace.Tracer
	provision metric.Float64Histogram
	checks    metric.Int64Counter
	score     metric.Float64Histogram
	exams     metric.Int64Counter
	examScore metric.Float64Histogram
	ratings   metric.Int64Histogram
	rejected  metric.Int64Counter
	termTime  metric.Float64Histogram
	storeTime metric.Float64Histogram
	terminals atomic.Int64
	meter     metric.Meter
}

// Endpoint returns the configured OTLP endpoint: the general one, or the
// first endpoint of a single signal. It is empty when telemetry is off.
func Endpoint() string {
	for _, k := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func endpointConfigured() bool { return Endpoint() != "" }

// Setup configures the global providers. base is the logger that keeps
// writing to stderr; the returned Telemetry.Logger additionally ships logs.
// A line written inside a span names it on stderr too (trace_id, span_id).
// Without telemetry no context has a span, and nothing is added.
func Setup(ctx context.Context, version string, base *slog.Logger) (*Telemetry, error) {
	stderr := traceHandler{base.Handler()}
	t := &Telemetry{Logger: slog.New(stderr)}
	if !endpointConfigured() {
		// The global providers do nothing until an SDK is installed.
		t.instruments(otel.GetTracerProvider(), otel.GetMeterProvider())
		return t, nil
	}
	if os.Getenv("OTEL_SERVICE_NAME") == "" {
		os.Setenv("OTEL_SERVICE_NAME", "kubedojo")
	}
	res, err := resource.New(ctx, resource.WithFromEnv(), resource.WithTelemetrySDK(), resource.WithHost(),
		resource.WithAttributes(attribute.String("service.version", version)))
	if err != nil && res == nil {
		return nil, err
	}

	traceExp, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(traceExp), sdktrace.WithResource(res))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	metricExp, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return nil, err
	}
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp, sdkmetric.WithInterval(15*time.Second))))
	otel.SetMeterProvider(mp)

	logExp, err := otlploghttp.New(ctx)
	if err != nil {
		return nil, err
	}
	lp := sdklog.NewLoggerProvider(sdklog.WithResource(res), sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)))

	t.Enabled = true
	t.shutdown = []func(context.Context) error{tp.Shutdown, mp.Shutdown, lp.Shutdown}
	t.Logger = slog.New(fanout{stderr, otelslog.NewHandler(scope, otelslog.WithLoggerProvider(lp))})
	t.instruments(tp, mp)
	return t, nil
}

// New returns a Telemetry that records to the given providers and logs to
// logger. Setup is what the server uses; this is for tests and for embedding
// kubedojo into a process that owns its OpenTelemetry SDK. As with Setup, a
// line of Telemetry.Logger written inside a span carries trace_id and span_id.
func New(tp trace.TracerProvider, mp metric.MeterProvider, logger *slog.Logger) *Telemetry {
	t := &Telemetry{Enabled: true, Logger: slog.New(traceHandler{logger.Handler()})}
	t.instruments(tp, mp)
	return t
}

func (t *Telemetry) instruments(tp trace.TracerProvider, mp metric.MeterProvider) {
	t.tp, t.mp = tp, mp
	t.tracer = tp.Tracer(scope)
	t.meter = mp.Meter(scope)
	t.rejected, _ = t.meter.Int64Counter("kubedojo.http.requests.rejected",
		metric.WithDescription("Requests refused before they reached the API, by reason."))
	t.termTime, _ = t.meter.Float64Histogram("kubedojo.terminal.session.duration", metric.WithUnit("s"),
		metric.WithDescription("How long a browser terminal stayed open."),
		metric.WithExplicitBucketBoundaries(1, 5, 15, 60, 300, 900, 1800, 3600, 7200, 14400))
	t.storeTime, _ = t.meter.Float64Histogram("kubedojo.store.operation.duration", metric.WithUnit("s"),
		metric.WithDescription("Time of one operation on the progress database, by operation."),
		metric.WithExplicitBucketBoundaries(0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5))
	t.provision, _ = t.meter.Float64Histogram("kubedojo.sandbox.provision.duration", metric.WithUnit("s"),
		metric.WithDescription("Time to create a sandbox cluster (practice) or a whole exam environment."),
		metric.WithExplicitBucketBoundaries(5, 10, 20, 30, 45, 60, 90, 120, 180, 300, 600))
	t.checks, _ = t.meter.Int64Counter("kubedojo.check.runs",
		metric.WithDescription("Graded questions by result."))
	t.score, _ = t.meter.Float64Histogram("kubedojo.question.score.ratio", metric.WithUnit("1"),
		metric.WithDescription("Share of points reached per graded question (0..1)."),
		metric.WithExplicitBucketBoundaries(0, 0.25, 0.5, 0.75, 0.99, 1))
	t.exams, _ = t.meter.Int64Counter("kubedojo.exams.finished",
		metric.WithDescription("Finished exam simulations by result."))
	t.examScore, _ = t.meter.Float64Histogram("kubedojo.exam.score", metric.WithUnit("%"),
		metric.WithDescription("Exam result in percent."),
		metric.WithExplicitBucketBoundaries(0, 10, 20, 30, 40, 50, 60, 66, 70, 80, 90, 100))
	t.ratings, _ = t.meter.Int64Histogram("kubedojo.question.rating",
		metric.WithDescription("Learner ratings per question (1-5 stars)."),
		metric.WithExplicitBucketBoundaries(1, 2, 3, 4, 5))
	_, _ = t.meter.Int64ObservableGauge("kubedojo.terminal.connections",
		metric.WithDescription("Open browser terminals."),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(t.terminals.Load())
			return nil
		}))
}

// ObserveSessions registers the gauge of live sessions.
func (t *Telemetry) ObserveSessions(count func() (practice, exam int)) {
	_, _ = t.meter.Int64ObservableGauge("kubedojo.sessions.active",
		metric.WithDescription("Live sessions (each owns a sandbox)."),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			p, e := count()
			o.Observe(int64(p), metric.WithAttributes(attribute.String("mode", "practice")))
			o.Observe(int64(e), metric.WithAttributes(attribute.String("mode", "exam")))
			return nil
		}))
}

// ObserveCapacity registers the gauges of the session limit: the sessions
// that own a sandbox, and how many may. Sessions of theory tracks are live
// sessions too (kubedojo.sessions.active) but take no slot.
func (t *Telemetry) ObserveCapacity(capacity func() (used, limit int)) {
	active, _ := t.meter.Int64ObservableGauge("kubedojo.sandboxes.active",
		metric.WithDescription("Sessions that own a sandbox. These count against the session limit."))
	limit, _ := t.meter.Int64ObservableGauge("kubedojo.sandboxes.limit",
		metric.WithDescription("How many sessions may own a sandbox at the same time (--max-sessions)."))
	_, _ = t.meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		u, l := capacity()
		o.ObserveInt64(active, int64(u))
		o.ObserveInt64(limit, int64(l))
		return nil
	}, active, limit)
}

// SandboxUsage is what the sandbox containers of one role use together.
type SandboxUsage struct {
	Containers  int
	MemoryBytes int64
	CPU         float64 // in cores
}

// ObserveSandboxes registers the gauges of a local kubedojo's sandbox
// containers. sample returns the last measurement by role (node, terminal,
// linux) and the memory Docker has; it must be cheap, the measuring happens
// elsewhere. In hosted mode sandboxes are Pods and their numbers come from
// the cluster, so these gauges are not registered there.
func (t *Telemetry) ObserveSandboxes(sample func() (byRole map[string]SandboxUsage, dockerMemory int64)) {
	containers, _ := t.meter.Int64ObservableGauge("kubedojo.sandbox.containers",
		metric.WithDescription("Running sandbox containers of a local kubedojo, by role."))
	memory, _ := t.meter.Int64ObservableGauge("kubedojo.sandbox.memory.usage", metric.WithUnit("By"),
		metric.WithDescription("Memory used by the sandbox containers of a local kubedojo, by role."))
	cpu, _ := t.meter.Float64ObservableGauge("kubedojo.sandbox.cpu.usage", metric.WithUnit("{cpu}"),
		metric.WithDescription("CPU cores used by the sandbox containers of a local kubedojo, by role."))
	limit, _ := t.meter.Int64ObservableGauge("kubedojo.docker.memory.limit", metric.WithUnit("By"),
		metric.WithDescription("Memory Docker has: the machine's, or the VM's with Docker Desktop."))
	_, _ = t.meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		byRole, total := sample()
		// Every role every time: a series that stops is a gap, not a zero.
		for _, role := range []string{"node", "terminal", "linux"} {
			u, a := byRole[role], metric.WithAttributes(attribute.String("role", role))
			o.ObserveInt64(containers, int64(u.Containers), a)
			o.ObserveInt64(memory, u.MemoryBytes, a)
			o.ObserveFloat64(cpu, u.CPU, a)
		}
		if total > 0 {
			o.ObserveInt64(limit, total)
		}
		return nil
	}, containers, memory, cpu, limit)
}

func result(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
}

// Span starts a span for a piece of session work. attrs are pairs of key and
// value. The returned function ends the span and records the error, if any.
func (t *Telemetry) Span(ctx context.Context, name string, attrs ...string) (context.Context, func(error)) {
	kv := make([]attribute.KeyValue, 0, len(attrs)/2)
	for i := 0; i+1 < len(attrs); i += 2 {
		kv = append(kv, attribute.String(attrs[i], attrs[i+1]))
	}
	ctx, span := t.tracer.Start(ctx, name, trace.WithAttributes(kv...))
	return ctx, func(err error) {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}
}

// Detach returns a context for work that outlives a request: it lives as long
// as base, and spans started in it belong to the trace of request. Creating a
// cluster takes a minute; the request that asked for it is answered at once.
// The tracer of the request travels along, so span.Start keeps working there.
func Detach(base, request context.Context) context.Context {
	ctx := trace.ContextWithSpanContext(base, trace.SpanContextFromContext(request))
	return span.With(ctx, span.From(request))
}

// Provisioned records one sandbox provisioning. Recorded inside the span of
// the provisioning, the sample carries its trace as an exemplar.
func (t *Telemetry) Provisioned(ctx context.Context, track, mode string, d time.Duration, err error) {
	t.provision.Record(ctx, d.Seconds(), metric.WithAttributes(
		attribute.String("track", track), attribute.String("mode", mode), attribute.String("result", result(err))))
}

// Checked records one graded question.
func (t *Telemetry) Checked(ctx context.Context, track, question, mode string, score, max int) {
	res := "failed"
	if score == max {
		res = "passed"
	}
	attrs := metric.WithAttributes(attribute.String("track", track), attribute.String("question", question),
		attribute.String("mode", mode), attribute.String("result", res))
	t.checks.Add(ctx, 1, attrs)
	if max > 0 {
		t.score.Record(ctx, float64(score)/float64(max), metric.WithAttributes(attribute.String("track", track), attribute.String("mode", mode)))
	}
}

// ExamDone records one finished exam.
func (t *Telemetry) ExamDone(ctx context.Context, track string, percent float64, passed bool) {
	res := "failed"
	if passed {
		res = "passed"
	}
	t.exams.Add(ctx, 1, metric.WithAttributes(attribute.String("track", track), attribute.String("result", res)))
	t.examScore.Record(ctx, percent, metric.WithAttributes(attribute.String("track", track)))
}

// Rated records a learner's rating of a question.
func (t *Telemetry) Rated(ctx context.Context, track, question string, stars int) {
	t.ratings.Record(ctx, int64(stars),
		metric.WithAttributes(attribute.String("track", track), attribute.String("question", question)))
}

// Terminal tracks open terminal connections.
func (t *Telemetry) Terminal(delta int) { t.terminals.Add(int64(delta)) }

// TerminalClosed records how long one browser terminal was open. Recorded in
// the context of its terminal.attach span, the sample carries that trace.
func (t *Telemetry) TerminalClosed(ctx context.Context, d time.Duration) {
	t.termTime.Record(ctx, d.Seconds())
}

// StoreOperation records one operation on the progress database. operation
// is one of the fixed names the store reports (store.Operations).
func (t *Telemetry) StoreOperation(operation string, d time.Duration) {
	t.storeTime.Record(context.Background(), d.Seconds(), metric.WithAttributes(attribute.String("operation", operation)))
}

// Rejected counts a request that was refused before it reached the API: a
// Host or an Origin that is not allowed, a body that is too large. reason is
// one of a small fixed set of words the caller chooses, never text taken
// from the request: it becomes a metric label.
func (t *Telemetry) Rejected(reason string) {
	t.rejected.Add(context.Background(), 1, metric.WithAttributes(attribute.String("reason", reason)))
}

// HTTP wraps the API handler with request spans and metrics. The terminal
// WebSocket and static assets are excluded to keep cardinality and noise low.
//
// The context of every request carries the tracer: a handler reaches the
// request span with span.Current and starts spans below it with span.Start.
// A traced request is answered with a Server-Timing header that names its
// trace in the traceparent format, so the UI can quote the trace of a
// request that failed.
func (t *Telemetry) HTTP(h http.Handler) http.Handler {
	if !t.Enabled {
		return h
	}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s := trace.SpanFromContext(r.Context()); s.IsRecording() {
			sc := s.SpanContext()
			w.Header().Add("Server-Timing", `traceparent;desc="00-`+sc.TraceID().String()+"-"+sc.SpanID().String()+"-"+sc.TraceFlags().String()+`"`)
		}
		req := r.WithContext(span.With(r.Context(), t))
		// The mux writes the route it matched to the request it is given.
		// otelhttp names the span and the metric after the route of its own
		// request, so the route goes back there, also when a handler panics.
		// otelhttp does not put the route on the span as an attribute when
		// it learns it this late; Grafana's link from a trace to the metrics
		// of its route reads http.route, so it is set here.
		defer func() {
			r.Pattern = req.Pattern
			if req.MultipartForm != nil {
				r.MultipartForm = req.MultipartForm
			}
			if i := strings.IndexByte(req.Pattern, '/'); i >= 0 {
				trace.SpanFromContext(r.Context()).SetAttributes(attribute.String("http.route", req.Pattern[i:]))
			}
		}()
		h.ServeHTTP(w, req)
	})
	return otelhttp.NewHandler(inner, "http",
		otelhttp.WithTracerProvider(t.tp), otelhttp.WithMeterProvider(t.mp),
		otelhttp.WithFilter(func(r *http.Request) bool {
			p := r.URL.Path
			return strings.HasPrefix(p, "/api/") && p != "/api/health" && p != "/api/ready" && r.Header.Get("Upgrade") == ""
		}),
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			if r.Pattern != "" {
				return r.Pattern
			}
			return r.Method + " " + r.URL.Path
		}))
}

// Shutdown flushes all exporters.
func (t *Telemetry) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var errs []error
	for _, f := range t.shutdown {
		errs = append(errs, f(ctx))
	}
	if err := errors.Join(errs...); err != nil {
		t.Logger.Warn("telemetry shutdown", "err", err)
	}
}

// fanout sends every record to several slog handlers.
type fanout []slog.Handler

func (f fanout) Enabled(ctx context.Context, l slog.Level) bool {
	for _, h := range f {
		if h.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (f fanout) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, h := range f {
		if h.Enabled(ctx, r.Level) {
			errs = append(errs, h.Handle(ctx, r.Clone()))
		}
	}
	return errors.Join(errs...)
}

func (f fanout) WithAttrs(a []slog.Attr) slog.Handler {
	out := make(fanout, len(f))
	for i, h := range f {
		out[i] = h.WithAttrs(a)
	}
	return out
}

func (f fanout) WithGroup(name string) slog.Handler {
	out := make(fanout, len(f))
	for i, h := range f {
		out[i] = h.WithGroup(name)
	}
	return out
}

// traceHandler adds the trace and the span of a record's context to the
// record, under the names Loki and Grafana know (trace_id, span_id). The
// OpenTelemetry log bridge reads them from the context itself; a handler
// that writes to stderr does not.
type traceHandler struct{ slog.Handler }

func (h traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r = r.Clone()
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	return h.Handler.Handle(ctx, r)
}

func (h traceHandler) WithAttrs(a []slog.Attr) slog.Handler {
	return traceHandler{h.Handler.WithAttrs(a)}
}

func (h traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{h.Handler.WithGroup(name)}
}
