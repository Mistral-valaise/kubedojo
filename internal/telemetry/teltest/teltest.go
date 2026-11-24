// Package teltest records what kubedojo traces, for tests: a real Telemetry on
// an in-memory OpenTelemetry SDK, and helpers to read the spans as the tree a
// person sees in Tempo.
package teltest

import (
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/Mistral-valaise/kubedojo/internal/telemetry"
)

// Recorder is a Telemetry whose spans stay in memory. It is a span.Tracer,
// like the Telemetry of the server.
type Recorder struct {
	*telemetry.Telemetry
	// Provider is the tracer provider the spans come from, for code that
	// takes one (an HTTP transport, a test's own request span).
	Provider *sdktrace.TracerProvider
	spans    *tracetest.SpanRecorder
}

// New returns a Recorder that records no metrics and logs nowhere.
func New() *Recorder {
	return NewWith(noopmetric.NewMeterProvider(), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// NewWith returns a Recorder that records metrics to mp and logs to logger.
func NewWith(mp metric.MeterProvider, logger *slog.Logger) *Recorder {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	return &Recorder{Telemetry: telemetry.New(tp, mp, logger), Provider: tp, spans: rec}
}

// Tracer returns a tracer of the recorder's provider, for a span a test
// starts itself, such as the span of a request.
func (r *Recorder) Tracer() trace.Tracer { return r.Provider.Tracer("test") }

// Ended returns the spans that have ended, in the order they ended.
func (r *Recorder) Ended() []sdktrace.ReadOnlySpan { return r.spans.Ended() }

// Named returns the ended spans with the name.
func (r *Recorder) Named(name string) []sdktrace.ReadOnlySpan {
	var out []sdktrace.ReadOnlySpan
	for _, s := range r.spans.Ended() {
		if s.Name() == name {
			out = append(out, s)
		}
	}
	return out
}

// Children returns the ended spans whose parent is the given span, by start.
func (r *Recorder) Children(parent sdktrace.ReadOnlySpan) []sdktrace.ReadOnlySpan {
	var out []sdktrace.ReadOnlySpan
	for _, s := range r.spans.Ended() {
		if s.Parent().SpanID() == parent.SpanContext().SpanID() && s.SpanContext().TraceID() == parent.SpanContext().TraceID() {
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartTime().Before(out[j].StartTime()) })
	return out
}

// Attrs returns the attributes of a span by key.
func Attrs(s sdktrace.ReadOnlySpan) map[string]attribute.Value {
	out := map[string]attribute.Value{}
	for _, a := range s.Attributes() {
		out[string(a.Key)] = a.Value
	}
	return out
}

// Events returns the events of a span with the name, each as its attributes.
func Events(s sdktrace.ReadOnlySpan, name string) []map[string]attribute.Value {
	var out []map[string]attribute.Value
	for _, e := range s.Events() {
		if e.Name != name {
			continue
		}
		m := map[string]attribute.Value{}
		for _, a := range e.Attributes {
			m[string(a.Key)] = a.Value
		}
		out = append(out, m)
	}
	return out
}

// Tree renders spans the way a trace view shows them: one line per span,
// children indented below their parent in the order they started, each with
// its status (only if it is an error), its attributes and its events. hide
// lists attribute keys to leave out, such as ids that differ on every run.
func Tree(spans []sdktrace.ReadOnlySpan, hide ...string) string {
	return render(spans, true, hide)
}

// Shape is Tree with the names of the spans only: which spans there are, and
// which is below which.
func Shape(spans []sdktrace.ReadOnlySpan) string { return render(spans, false, nil) }

// InTrace returns the spans of one trace.
func InTrace(spans []sdktrace.ReadOnlySpan, id trace.TraceID) []sdktrace.ReadOnlySpan {
	var out []sdktrace.ReadOnlySpan
	for _, s := range spans {
		if s.SpanContext().TraceID() == id {
			out = append(out, s)
		}
	}
	return out
}

func render(spans []sdktrace.ReadOnlySpan, detail bool, hide []string) string {
	hidden := map[string]bool{}
	for _, k := range hide {
		hidden[k] = true
	}
	known := map[trace.SpanID]bool{}
	for _, s := range spans {
		known[s.SpanContext().SpanID()] = true
	}
	children := map[trace.SpanID][]sdktrace.ReadOnlySpan{}
	var roots []sdktrace.ReadOnlySpan
	for _, s := range spans {
		if p := s.Parent().SpanID(); s.Parent().IsValid() && known[p] {
			children[p] = append(children[p], s)
		} else {
			roots = append(roots, s)
		}
	}
	byStart := func(l []sdktrace.ReadOnlySpan) {
		sort.SliceStable(l, func(i, j int) bool { return l[i].StartTime().Before(l[j].StartTime()) })
	}
	pairs := func(kv []attribute.KeyValue) string {
		var parts []string
		for _, a := range kv {
			if !hidden[string(a.Key)] {
				parts = append(parts, fmt.Sprintf("%s=%s", a.Key, a.Value.String()))
			}
		}
		sort.Strings(parts)
		return strings.Join(parts, " ")
	}
	var b strings.Builder
	var walk func(s sdktrace.ReadOnlySpan, depth int)
	walk = func(s sdktrace.ReadOnlySpan, depth int) {
		pad := strings.Repeat("   ", depth)
		fmt.Fprintf(&b, "%s%s", pad, s.Name())
		if st := s.Status(); detail && st.Code == codes.Error {
			fmt.Fprintf(&b, "  [ERROR: %s]", st.Description)
		}
		if a := pairs(s.Attributes()); detail && a != "" {
			fmt.Fprintf(&b, "  {%s}", a)
		}
		b.WriteString("\n")
		for _, e := range s.Events() {
			if !detail || e.Name == "exception" {
				continue // the error status above says it
			}
			fmt.Fprintf(&b, "%s   · %s", pad, e.Name)
			if a := pairs(e.Attributes); a != "" {
				fmt.Fprintf(&b, " {%s}", a)
			}
			b.WriteString("\n")
		}
		kids := children[s.SpanContext().SpanID()]
		byStart(kids)
		for _, c := range kids {
			walk(c, depth+1)
		}
	}
	byStart(roots)
	for _, r := range roots {
		walk(r, 0)
	}
	return b.String()
}
