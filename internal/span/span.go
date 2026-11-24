// Package span is the tracing API of kubedojo's own code: spans with typed
// attributes and events, and a tracer that travels in the context. It imports
// no OpenTelemetry; internal/telemetry implements it. Without a tracer in the
// context every call is a no-op, so code can trace without checking.
package span

import "context"

// Attr is one attribute of a span or of an event. Value is a string, bool,
// int, int64, float64 or []string; the constructors below build each.
type Attr struct {
	Key   string
	Value any
}

func String(k, v string) Attr           { return Attr{k, v} }
func Int(k string, v int) Attr          { return Attr{k, v} }
func Int64(k string, v int64) Attr      { return Attr{k, v} }
func Bool(k string, v bool) Attr        { return Attr{k, v} }
func Float(k string, v float64) Attr    { return Attr{k, v} }
func Strings(k string, v []string) Attr { return Attr{k, v} }

// Span is one piece of work in a trace.
type Span interface {
	// Set adds attributes; an attribute set again replaces the earlier one.
	Set(attrs ...Attr)
	// Event records that something happened at this moment.
	Event(name string, attrs ...Attr)
	// End ends the span. It records the error and sets the error status
	// when err != nil.
	End(err error)
}

// Tracer starts spans.
type Tracer interface {
	// Start starts a span as a child of the span in ctx. The returned context
	// carries the new span, and the tracer itself.
	Start(ctx context.Context, name string, attrs ...Attr) (context.Context, Span)
	// Current returns the span active in ctx; a no-op span if there is none.
	Current(ctx context.Context) Span
}

type key struct{}

// With returns a context that carries the tracer. A nil tracer leaves the
// context as it is.
func With(ctx context.Context, t Tracer) context.Context {
	if t == nil {
		return ctx
	}
	return context.WithValue(ctx, key{}, t)
}

// From returns the tracer of the context; a no-op Tracer if it has none.
func From(ctx context.Context) Tracer {
	if t, ok := ctx.Value(key{}).(Tracer); ok {
		return t
	}
	return noopTracer{}
}

// Start starts a span with the tracer of the context.
func Start(ctx context.Context, name string, attrs ...Attr) (context.Context, Span) {
	return From(ctx).Start(ctx, name, attrs...)
}

// Current returns the span active in ctx, as the tracer of the context sees
// it; a no-op span if there is none.
func Current(ctx context.Context) Span {
	return From(ctx).Current(ctx)
}

// TraceID returns the id of the trace ctx is in, when the tracer of the
// context can tell and the trace is being recorded; otherwise it is empty.
// The API hands it to the client next to a result, so that a person can look
// the trace up.
func TraceID(ctx context.Context) string {
	if t, ok := From(ctx).(interface {
		TraceID(context.Context) string
	}); ok {
		return t.TraceID(ctx)
	}
	return ""
}

type noopTracer struct{}

func (noopTracer) Start(ctx context.Context, _ string, _ ...Attr) (context.Context, Span) {
	return ctx, noopSpan{}
}
func (noopTracer) Current(context.Context) Span { return noopSpan{} }

type noopSpan struct{}

func (noopSpan) Set(...Attr)           {}
func (noopSpan) Event(string, ...Attr) {}
func (noopSpan) End(error)             {}
