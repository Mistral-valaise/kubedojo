package span

import (
	"context"
	"errors"
	"testing"
)

// Without a tracer everything is a no-op that is safe to call: code traces
// without asking whether tracing is on.
func TestNoTracerIsSafe(t *testing.T) {
	ctx := context.Background()
	if From(ctx) == nil {
		t.Fatal("From without a tracer = nil, want a no-op tracer")
	}
	got, s := Start(ctx, "work", String("k", "v"), Int("n", 1))
	if got != ctx {
		t.Error("the no-op tracer changed the context")
	}
	s.Set(Bool("b", true), Float("f", 1.5), Int64("i", 2), Strings("s", []string{"a", "b"}))
	s.Set()
	s.Event("happened", String("k", "v"))
	s.End(errors.New("failed"))
	s.End(nil) // ending twice is harmless too

	cur := Current(ctx)
	cur.Set(String("k", "v"))
	cur.Event("happened")
	cur.End(nil)

	if With(ctx, nil) != ctx {
		t.Error("With(nil) changed the context")
	}
	if _, ok := From(With(ctx, nil)).(noopTracer); !ok {
		t.Error("From after With(nil) is not the no-op tracer")
	}
}

// recorder is a Tracer that keeps what it was told.
type recorder struct {
	started []string
	attrs   map[string]any
	events  []string
	ended   []error
}

type recorded struct{ r *recorder }

type spanKey struct{}

func (r *recorder) Start(ctx context.Context, name string, attrs ...Attr) (context.Context, Span) {
	r.started = append(r.started, name)
	s := recorded{r}
	s.Set(attrs...)
	return context.WithValue(With(ctx, r), spanKey{}, s), s
}

func (r *recorder) Current(ctx context.Context) Span {
	if s, ok := ctx.Value(spanKey{}).(Span); ok {
		return s
	}
	return noopSpan{}
}

func (s recorded) Set(attrs ...Attr) {
	for _, a := range attrs {
		s.r.attrs[a.Key] = a.Value
	}
}
func (s recorded) Event(name string, _ ...Attr) { s.r.events = append(s.r.events, name) }
func (s recorded) End(err error)                { s.r.ended = append(s.r.ended, err) }

// The tracer put into a context is the one From returns and the one the
// package functions use.
func TestWithFromRoundTrip(t *testing.T) {
	r := &recorder{attrs: map[string]any{}}
	ctx := With(context.Background(), r)
	if From(ctx) != Tracer(r) {
		t.Fatal("From does not return the tracer given to With")
	}
	type other struct{}
	if From(context.WithValue(ctx, other{}, 1)) != Tracer(r) {
		t.Error("a derived context lost the tracer")
	}

	Current(ctx).Set(String("early", "there is no span yet")) // a no-op
	if len(r.attrs) != 0 {
		t.Errorf("attributes without a span: %v", r.attrs)
	}

	ctx, s := Start(ctx, "work", String("track", "cka"), Int("questions", 17))
	Current(ctx).Set(Bool("exam", true))
	Current(ctx).Event("ready")
	failure := errors.New("failed")
	s.End(failure)

	if len(r.started) != 1 || r.started[0] != "work" {
		t.Errorf("started = %v, want [work]", r.started)
	}
	if r.attrs["track"] != "cka" || r.attrs["questions"] != 17 || r.attrs["exam"] != true {
		t.Errorf("attributes = %v", r.attrs)
	}
	if len(r.events) != 1 || r.events[0] != "ready" {
		t.Errorf("events = %v, want [ready]", r.events)
	}
	if len(r.ended) != 1 || r.ended[0] != failure {
		t.Errorf("ended = %v, want the error", r.ended)
	}
}

// Each constructor keeps the Go type of its value: an implementation maps
// that type to the attribute type of its backend.
func TestAttrTypes(t *testing.T) {
	for _, c := range []struct {
		a    Attr
		want string
	}{
		{String("k", "v"), "string"},
		{Int("k", 1), "int"},
		{Int64("k", 1), "int64"},
		{Bool("k", true), "bool"},
		{Float("k", 1), "float64"},
		{Strings("k", []string{"a"}), "[]string"},
	} {
		var got string
		switch c.a.Value.(type) {
		case string:
			got = "string"
		case int:
			got = "int"
		case int64:
			got = "int64"
		case bool:
			got = "bool"
		case float64:
			got = "float64"
		case []string:
			got = "[]string"
		}
		if got != c.want || c.a.Key != "k" {
			t.Errorf("%#v: value type %s, want %s", c.a, got, c.want)
		}
	}
}
