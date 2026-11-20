package telemetry

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Names of the canary, as the backends know them.
const (
	CanaryService = "kubedojo-verify"
	CanaryMetric  = "kubedojo_verify_runs_total"
	CanaryMessage = "kubedojo verify canary"
)

// Canary sends one span, one log record written inside it and one counter
// increment recorded inside it to an OTLP/HTTP endpoint, and waits until the
// endpoint has taken them. It returns the trace ID the three share.
//
// It uses providers of its own and the service name kubedojo-verify, so the
// canary never shows up among kubedojo's telemetry.
func Canary(ctx context.Context, endpoint string) (traceID string, err error) {
	res, err := resource.New(ctx, resource.WithAttributes(attribute.String("service.name", CanaryService)))
	if err != nil {
		return "", err
	}
	traceExp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint+"/v1/traces"))
	if err != nil {
		return "", err
	}
	metricExp, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(endpoint+"/v1/metrics"))
	if err != nil {
		return "", err
	}
	logExp, err := otlploghttp.New(ctx, otlploghttp.WithEndpointURL(endpoint+"/v1/logs"))
	if err != nil {
		return "", err
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(traceExp), sdktrace.WithResource(res))
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithResource(res), sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)))
	lp := sdklog.NewLoggerProvider(sdklog.WithResource(res), sdklog.WithProcessor(sdklog.NewSimpleProcessor(logExp)))

	counter, err := mp.Meter(scope).Int64Counter("kubedojo.verify.runs")
	if err != nil {
		return "", err
	}
	sctx, span := tp.Tracer(scope).Start(ctx, "kubedojo.verify")
	traceID = span.SpanContext().TraceID().String()
	counter.Add(sctx, 1)
	var rec log.Record
	rec.SetTimestamp(time.Now())
	rec.SetSeverity(log.SeverityInfo)
	rec.SetSeverityText("INFO")
	rec.SetBody(attribute.StringValue(CanaryMessage))
	lp.Logger(scope).Emit(sctx, rec)
	span.End()

	// Shutdown flushes: an error here is the endpoint not taking the signal.
	flush, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return traceID, errors.Join(tp.Shutdown(flush), mp.Shutdown(flush), lp.Shutdown(flush))
}
