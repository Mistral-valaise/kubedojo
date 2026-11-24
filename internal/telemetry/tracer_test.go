package telemetry

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/Mistral-valaise/kubedojo/internal/span"
)

func recordingTelemetry() (*Telemetry, *tracetest.SpanRecorder) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	return New(tp, noopmetric.NewMeterProvider(), slog.New(slog.NewTextHandler(io.Discard, nil))), rec
}

func ended(t *testing.T, rec *tracetest.SpanRecorder, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, s := range rec.Ended() {
		if s.Name() == name {
			return s
		}
	}
	t.Fatalf("no ended span named %q", name)
	return nil
}

func attrs(kv []attribute.KeyValue) map[string]attribute.Value {
	out := map[string]attribute.Value{}
	for _, a := range kv {
		out[string(a.Key)] = a.Value
	}
	return out
}

// Telemetry is the span.Tracer of the server: attributes keep their type, an
// event is recorded, and a failure ends the span with the error status.
func TestTracerRecordsTypedSpans(t *testing.T) {
	tel, rec := recordingTelemetry()
	ctx, s := tel.Start(context.Background(), "work",
		span.String("track", "cka"), span.Int("questions", 17), span.Int64("bytes", 1<<40),
		span.Bool("exam", true), span.Float("ratio", 0.5), span.Strings("hosts", []string{"a", "b"}))
	s.Set(span.Int("questions", 18), span.String("mode", "exam"))
	// The span is reached through the context as well.
	span.Current(ctx).Set(span.Bool("late", true))
	span.Current(ctx).Event("ready", span.Int("attempt", 2), span.String("host", "a"))
	failure := errors.New("the cluster did not come up")
	s.End(failure)

	got := ended(t, rec, "work")
	a := attrs(got.Attributes())
	for key, want := range map[string]attribute.Value{
		"track":     attribute.StringValue("cka"),
		"questions": attribute.IntValue(18),
		"bytes":     attribute.Int64Value(1 << 40),
		"exam":      attribute.BoolValue(true),
		"ratio":     attribute.Float64Value(0.5),
		"hosts":     attribute.StringSliceValue([]string{"a", "b"}),
		"mode":      attribute.StringValue("exam"),
		"late":      attribute.BoolValue(true),
	} {
		if a[key].Type() != want.Type() || a[key].String() != want.String() {
			t.Errorf("attribute %s = %s %s, want %s %s", key, a[key].Type(), a[key].String(), want.Type(), want.String())
		}
	}

	events := map[string]map[string]attribute.Value{}
	for _, e := range got.Events() {
		events[e.Name] = attrs(e.Attributes)
	}
	ready, ok := events["ready"]
	if !ok {
		t.Fatalf("no event \"ready\"; events: %v", events)
	}
	if ready["attempt"].Type() != attribute.INT64 || ready["attempt"].AsInt64() != 2 || ready["host"].AsString() != "a" {
		t.Errorf("event attributes = %v", ready)
	}
	// RecordError keeps the error as an event of its own.
	if exc, ok := events["exception"]; !ok || exc["exception.message"].AsString() != failure.Error() {
		t.Errorf("the error was not recorded; events: %v", events)
	}
	if st := got.Status(); st.Code != codes.Error || st.Description != failure.Error() {
		t.Errorf("status = %v %q, want Error %q", st.Code, st.Description, failure.Error())
	}
}

// A span that ends without an error has no error status, and a span started
// through the context is a child in the same trace.
func TestTracerTravelsInTheContext(t *testing.T) {
	tel, rec := recordingTelemetry()
	ctx, parent := tel.Start(context.Background(), "parent")
	if span.From(ctx) != span.Tracer(tel) {
		t.Fatal("the context of a started span does not carry the tracer")
	}
	_, child := span.Start(ctx, "child", span.String("k", "v"))
	child.End(nil)
	parent.End(nil)

	p, c := ended(t, rec, "parent"), ended(t, rec, "child")
	if c.Parent().SpanID() != p.SpanContext().SpanID() || c.SpanContext().TraceID() != p.SpanContext().TraceID() {
		t.Errorf("child %v is not a child of %v", c.Parent(), p.SpanContext())
	}
	if p.Status().Code != codes.Unset || c.Status().Code != codes.Unset {
		t.Errorf("status = %v and %v, want Unset", p.Status().Code, c.Status().Code)
	}
	if len(p.Events()) != 0 {
		t.Errorf("events without an error: %v", p.Events())
	}
}

// Without a span in the context, Current is a span that records nothing.
func TestTracerCurrentWithoutSpan(t *testing.T) {
	tel, rec := recordingTelemetry()
	cur := tel.Current(context.Background())
	cur.Set(span.String("k", "v"))
	cur.Event("happened")
	cur.End(errors.New("failed"))
	if n := len(rec.Ended()); n != 0 {
		t.Errorf("%d spans recorded, want none", n)
	}
}
