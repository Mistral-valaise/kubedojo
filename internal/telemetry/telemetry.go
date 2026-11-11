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

// Enabled reports whether an OTLP endpoint is configured.
func endpointConfigured() bool {
	for _, k := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT"} {
		if os.Getenv(k) != "" {
			return true
		}
	}
	return false
}

// Setup configures the global providers. base is the logger that keeps
// writing to stderr; the returned Telemetry.Logger additionally ships logs.
func Setup(ctx context.Context, version string, base *slog.Logger) (*Telemetry, error) {
	t := &Telemetry{Logger: base}
	if !endpointConfigured() {
		t.instruments()
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
	t.instruments()
	return t, nil
}

func (t *Telemetry) instruments() {
	t.tracer = otel.Tracer(scope)
	t.meter = otel.Meter(scope)
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

func result(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
}

// Provisioned records one sandbox provisioning.
func (t *Telemetry) Provisioned(track, mode string, d time.Duration, err error) {
	ctx := context.Background()
	attrs := []attribute.KeyValue{attribute.String("track", track), attribute.String("mode", mode), attribute.String("result", result(err))}
	t.provision.Record(ctx, d.Seconds(), metric.WithAttributes(attrs...))
	end := time.Now()
	_, span := t.tracer.Start(ctx, "sandbox.provision", trace.WithTimestamp(end.Add(-d)), trace.WithAttributes(attrs...))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End(trace.WithTimestamp(end))
}

// Checked records one graded question.
func (t *Telemetry) Checked(track, question, mode string, score, max int) {
	ctx := context.Background()
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
func (t *Telemetry) ExamDone(track string, percent float64, passed bool) {
	ctx := context.Background()
	res := "failed"
	if passed {
		res = "passed"
	}
	t.exams.Add(ctx, 1, metric.WithAttributes(attribute.String("track", track), attribute.String("result", res)))
	t.examScore.Record(ctx, percent, metric.WithAttributes(attribute.String("track", track)))
}

// Rated records a learner's rating of a question.
func (t *Telemetry) Rated(track, question string, stars int) {
	t.ratings.Record(context.Background(), int64(stars),
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
			return len(p) >= 5 && p[:5] == "/api/" && p != "/api/health" && r.Header.Get("Upgrade") == ""
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
