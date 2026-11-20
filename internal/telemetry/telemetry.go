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
)

const scope = "github.com/Mistral-valaise/kubedojo"

// Telemetry holds the instruments kubedojo records to.
type Telemetry struct {
	Enabled  bool
	Logger   *slog.Logger
	shutdown []func(context.Context) error

	tracer    trace.Tracer
	provision metric.Float64Histogram
	checks    metric.Int64Counter
	score     metric.Float64Histogram
	exams     metric.Int64Counter
	examScore metric.Float64Histogram
	ratings   metric.Int64Histogram
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
func Setup(ctx context.Context, version string, base *slog.Logger) (*Telemetry, error) {
	t := &Telemetry{Logger: base}
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
	t.Logger = slog.New(fanout{base.Handler(), otelslog.NewHandler(scope, otelslog.WithLoggerProvider(lp))})
	t.instruments(tp, mp)
	return t, nil
}

// New returns a Telemetry that records to the given providers and logs to
// logger. Setup is what the server uses; this is for tests and for embedding
// kubedojo into a process that owns its OpenTelemetry SDK.
func New(tp trace.TracerProvider, mp metric.MeterProvider, logger *slog.Logger) *Telemetry {
	t := &Telemetry{Enabled: true, Logger: logger}
	t.instruments(tp, mp)
	return t
}

func (t *Telemetry) instruments(tp trace.TracerProvider, mp metric.MeterProvider) {
	t.tracer = tp.Tracer(scope)
	t.meter = mp.Meter(scope)
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
func Detach(base, request context.Context) context.Context {
	return trace.ContextWithSpanContext(base, trace.SpanContextFromContext(request))
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

// HTTP wraps the API handler with request spans and metrics. The terminal
// WebSocket and static assets are excluded to keep cardinality and noise low.
func (t *Telemetry) HTTP(h http.Handler) http.Handler {
	if !t.Enabled {
		return h
	}
	return otelhttp.NewHandler(h, "http",
		otelhttp.WithFilter(func(r *http.Request) bool {
			p := r.URL.Path
			return len(p) >= 5 && p[:5] == "/api/" && p != "/api/health" && p != "/api/ready" && r.Header.Get("Upgrade") == ""
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
