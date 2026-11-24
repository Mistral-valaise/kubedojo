package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Mistral-valaise/kubedojo/internal/span"
)

var _ span.Tracer = (*Telemetry)(nil)

// Start starts a span with typed attributes. The returned context carries the
// span and the tracer, so that code further down traces with span.Start and
// span.Current without knowing this package.
func (t *Telemetry) Start(ctx context.Context, name string, attrs ...span.Attr) (context.Context, span.Span) {
	ctx, s := t.tracer.Start(ctx, name, trace.WithAttributes(keyValues(attrs)...))
	return span.With(ctx, t), otelSpan{s}
}

// Current returns the span active in ctx. Without one it is a span that
// records nothing.
func (t *Telemetry) Current(ctx context.Context) span.Span {
	return otelSpan{trace.SpanFromContext(ctx)}
}

// TraceID returns the id of the trace ctx is in, as 32 hex digits, when a
// span is recording there; otherwise it is empty. It is what span.TraceID
// reports to code that does not know OpenTelemetry.
func (t *Telemetry) TraceID(ctx context.Context) string {
	s := trace.SpanFromContext(ctx)
	if !s.IsRecording() || !s.SpanContext().HasTraceID() {
		return ""
	}
	return s.SpanContext().TraceID().String()
}

// otelSpan is an OpenTelemetry span behind the span.Span interface.
type otelSpan struct{ s trace.Span }

func (o otelSpan) Set(attrs ...span.Attr) { o.s.SetAttributes(keyValues(attrs)...) }

func (o otelSpan) Event(name string, attrs ...span.Attr) {
	o.s.AddEvent(name, trace.WithAttributes(keyValues(attrs)...))
}

// End ends the span. An error is recorded as an exception event and sets the
// error status; without one the status stays unset.
func (o otelSpan) End(err error) {
	if err != nil {
		o.s.RecordError(err)
		o.s.SetStatus(codes.Error, err.Error())
	}
	o.s.End()
}

// keyValues maps attributes to their OpenTelemetry type. A value of a type
// span.Attr does not name is recorded as its text.
func keyValues(attrs []span.Attr) []attribute.KeyValue {
	kv := make([]attribute.KeyValue, 0, len(attrs))
	for _, a := range attrs {
		switch v := a.Value.(type) {
		case string:
			kv = append(kv, attribute.String(a.Key, v))
		case bool:
			kv = append(kv, attribute.Bool(a.Key, v))
		case int:
			kv = append(kv, attribute.Int(a.Key, v))
		case int64:
			kv = append(kv, attribute.Int64(a.Key, v))
		case float64:
			kv = append(kv, attribute.Float64(a.Key, v))
		case []string:
			kv = append(kv, attribute.StringSlice(a.Key, v))
		default:
			kv = append(kv, attribute.String(a.Key, fmt.Sprint(v)))
		}
	}
	return kv
}
