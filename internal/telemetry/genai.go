package telemetry

import (
	"context"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/Mistral-valaise/kubedojo/internal/ai"
	"github.com/Mistral-valaise/kubedojo/internal/span"
)

// Limits of recorded content.
const (
	// ContentCap is how much text one content attribute of a model call
	// holds: the system instructions, the input messages together, and the
	// output. What is beyond it is cut.
	ContentCap = 32 << 10
	// ContentTruncated ends a text that was cut.
	ContentTruncated = "… [truncated]"
	// contentTruncatedAttr says a content attribute of the span was cut.
	contentTruncatedAttr = "kubedojo.assistant.content.truncated"
	// outputBuffer bounds what is kept of an answer before it is redacted
	// and cut to ContentCap.
	outputBuffer = 8 * ContentCap
)

// genaiInstruments registers the client metrics of the GenAI conventions.
func (t *Telemetry) genaiInstruments() {
	t.genDuration, _ = t.meter.Float64Histogram(genaiMetricDuration, metric.WithUnit("s"),
		metric.WithDescription("GenAI client inference operation duration."),
		metric.WithExplicitBucketBoundaries(genaiDurationBuckets...))
	t.genFirstChunk, _ = t.meter.Float64Histogram(genaiMetricTimeToFirstChunk, metric.WithUnit("s"),
		metric.WithDescription("Time from the start of an inference operation to the first chunk of its streaming response."),
		metric.WithExplicitBucketBoundaries(genaiDurationBuckets...))
	counter := func(name, desc string) metric.Int64Counter {
		c, _ := t.meter.Int64Counter(name, metric.WithUnit(genaiUnitToken), metric.WithDescription(desc))
		return c
	}
	t.genInput = counter(genaiMetricInputTokens, "The number of input (prompt) tokens used, including cached tokens.")
	t.genOutput = counter(genaiMetricOutputTokens, "The number of output (completion) tokens used.")
	t.genCacheRead = counter(genaiMetricCacheReadTokens, "The number of input tokens served from a provider-managed cache.")
	t.genCacheWrite = counter(genaiMetricCacheWriteTokens, "The number of input tokens written to a provider-managed cache.")
	t.genOpInput, _ = t.meter.Int64Histogram(genaiMetricOperationInputTokens, metric.WithUnit(genaiUnitToken),
		metric.WithDescription("The number of input (prompt) tokens used per inference operation."),
		metric.WithExplicitBucketBoundaries(genaiTokenBuckets...))
	t.genOpOutput, _ = t.meter.Int64Histogram(genaiMetricOperationOutputTokens, metric.WithUnit(genaiUnitToken),
		metric.WithDescription("The number of output (completion) tokens used per inference operation."),
		metric.WithExplicitBucketBoundaries(genaiTokenBuckets...))
}

// InstrumentProvider wraps a provider so that every call to the model is one
// span and one set of metric samples, named after the OpenTelemetry GenAI
// semantic conventions (genaiconv.go). The span is the same for every
// provider: what an adapter knows about its API it says through ai.Result.
//
// content says, per call, whether the prompt and the answer may be recorded.
// When it does not, the span has no attribute that holds any of them. What
// is recorded goes through Telemetry.Redact first, and without a Redact
// nothing is recorded.
func (t *Telemetry) InstrumentProvider(p ai.Provider, content func() bool) ai.Provider {
	if p == nil || !t.Enabled {
		return p
	}
	return &genaiProvider{Provider: p, t: t, content: content}
}

// ModelTransport wraps the HTTP transport model calls are made with: every
// request is a client span below the span of its call.
//
// No trace context is sent along. A model provider is a third party: it gets
// neither traceparent nor baggage, whatever propagator the process uses
// elsewhere. The span is named after the method and the host, and the client
// metrics of the HTTP instrumentation are not recorded.
func (t *Telemetry) ModelTransport(base http.RoundTripper) http.RoundTripper {
	if !t.Enabled {
		return base
	}
	return otelhttp.NewTransport(base,
		otelhttp.WithTracerProvider(t.tp),
		otelhttp.WithMeterProvider(noop.NewMeterProvider()),
		// An empty propagator, not nil: nil would mean the global one.
		otelhttp.WithPropagators(propagation.NewCompositeTextMapPropagator()),
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string { return r.Method + " " + r.URL.Host }))
}

// genaiProvider is a provider whose calls are traced and measured.
type genaiProvider struct {
	ai.Provider
	t       *Telemetry
	content func() bool
}

// Unwrap returns the provider that makes the calls.
func (g *genaiProvider) Unwrap() ai.Provider { return g.Provider }

// outputLimiter is a provider that says what output limit it asks for.
type outputLimiter interface {
	MaxOutputTokens(req ai.Request) int
}

// Chat makes the call inside the span "chat <model>".
func (g *genaiProvider) Chat(ctx context.Context, req ai.Request, onDelta func(string) error) (ai.Result, error) {
	caller, start := ctx, time.Now()
	model := g.Provider.Model()
	// What every sample of this call is labelled with. None of it comes
	// from a learner: the operator configures the provider and the model.
	labels := []attribute.KeyValue{
		attribute.String(genaiOperationName, genaiOperationChat),
		attribute.String(genaiProviderName, g.Provider.Name()),
		attribute.String(genaiRequestModel, model),
	}
	if host, port := g.Provider.Server(); host != "" {
		labels = append(labels, attribute.String(serverAddress, host), attribute.Int(serverPort, port))
	}
	// The adapters stream every answer.
	opening := append(labels[:len(labels):len(labels)], attribute.Bool(genaiRequestStream, true))
	limit := req.MaxOutputTokens
	if l, ok := g.Provider.(outputLimiter); ok {
		limit = l.MaxOutputTokens(req)
	}
	if limit > 0 {
		opening = append(opening, attribute.Int(genaiRequestMaxTokens, limit))
	}
	if req.ConversationID != "" {
		opening = append(opening, attribute.String(genaiConversationID, req.ConversationID))
	}
	ctx, sp := g.t.tracer.Start(ctx, genaiOperationChat+" "+model, trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(opening...))
	defer sp.End()
	ctx = span.With(ctx, g.t)

	redact := g.t.Redact
	capture := sp.IsRecording() && redact != nil && g.content != nil && g.content()
	truncated := false
	if capture {
		if strings.TrimSpace(req.System) != "" {
			text, cut := contentText(redact, req.System, ContentCap)
			truncated = truncated || cut
			sp.SetAttributes(attribute.KeyValue{Key: genaiSystemInstructions, Value: attribute.SliceValue(textPart(text))})
		}
		msgs, cut := inputMessages(redact, req.Messages)
		truncated = truncated || cut
		sp.SetAttributes(attribute.KeyValue{Key: genaiInputMessages, Value: msgs})
	}

	var (
		first    time.Duration
		answer   strings.Builder
		overflow bool
	)
	res, err := g.Provider.Chat(ctx, req, func(text string) error {
		if text != "" {
			if first == 0 {
				first = max(time.Since(start), time.Nanosecond)
				sp.SetAttributes(attribute.Float64(genaiResponseTimeToFirstChunk, first.Seconds()))
			}
			if capture {
				// Only the copy for the span is bounded: the caller gets
				// every piece as it came.
				keep := text
				if room := outputBuffer - answer.Len(); len(keep) > room {
					overflow = true
					keep = keep[:max(room, 0)]
				}
				answer.WriteString(keep)
			}
		}
		return onDelta(text)
	})
	elapsed := time.Since(start)

	// The outcome. A fault of the call is an error of the span. A caller
	// that went away and a model that declined are not: the server did its
	// work, and the span says what happened instead.
	var fault ai.ErrorKind
	finish := res.FinishReason
	began := first > 0 || res.ID != ""
	if err != nil {
		aerr := ai.AsError(err)
		finish = ""
		switch {
		case aerr.Kind == ai.ErrCanceled || caller.Err() == context.Canceled:
			sp.SetAttributes(attribute.String("kubedojo.refused", "cancelled"))
		case aerr.Kind == ai.ErrRefused:
			finish = genaiFinishContentFilter
		default:
			fault = aerr.Kind
			sp.SetAttributes(attribute.String(errorType, string(fault)))
			// The message is the one the learner sees: it never quotes the
			// upstream answer, the request or a credential.
			sp.SetStatus(codes.Error, aerr.Message)
		}
		// A generation that began and did not end properly ended in an error.
		if finish == "" && began {
			finish = genaiFinishError
		}
	}
	if finish != "" {
		sp.SetAttributes(attribute.StringSlice(genaiResponseFinishReasons, []string{finish}))
	}
	if res.ID != "" {
		sp.SetAttributes(attribute.String(genaiResponseID, res.ID))
	}
	// The model that answered. It can differ from the one asked for: an
	// alias resolves to a version, a fallback model takes over.
	if res.Model != "" && (err == nil || began) {
		labels = append(labels, attribute.String(genaiResponseModel, res.Model))
		sp.SetAttributes(attribute.String(genaiResponseModel, res.Model))
	}
	u := res.Usage
	if u.InputTokens > 0 || u.OutputTokens > 0 {
		sp.SetAttributes(attribute.Int64(genaiUsageInputTokens, u.InputTokens), attribute.Int64(genaiUsageOutputTokens, u.OutputTokens))
	}
	if u.CacheReadTokens > 0 {
		sp.SetAttributes(attribute.Int64(genaiUsageCacheReadTokens, u.CacheReadTokens))
	}
	if u.CacheWriteTokens > 0 {
		sp.SetAttributes(attribute.Int64(genaiUsageCacheWriteTokens, u.CacheWriteTokens))
	}
	if capture {
		// Output is recorded only if there was some, also of a call that
		// failed half way.
		if answer.Len() > 0 {
			text, cut := contentText(redact, answer.String(), ContentCap)
			if overflow && !cut {
				// The buffer ended anywhere, perhaps inside a character.
				text, cut = strings.ToValidUTF8(text, "")+ContentTruncated, true
			}
			truncated = truncated || cut
			sp.SetAttributes(attribute.KeyValue{Key: genaiOutputMessages, Value: attribute.SliceValue(message("assistant", text))})
		}
		if truncated {
			sp.SetAttributes(attribute.Bool(contentTruncatedAttr, true))
		}
	}

	// The samples are recorded inside the span, so each carries its trace as
	// an exemplar.
	plain := metric.WithAttributeSet(attribute.NewSet(labels...))
	outcome := plain
	if fault != "" {
		outcome = metric.WithAttributeSet(attribute.NewSet(append(labels[:len(labels):len(labels)], attribute.String(errorType, string(fault)))...))
	}
	g.t.genDuration.Record(ctx, elapsed.Seconds(), outcome)
	if first > 0 {
		g.t.genFirstChunk.Record(ctx, first.Seconds(), plain)
	}
	// Every call, with 0 when the provider reported nothing.
	g.t.genOpInput.Record(ctx, u.InputTokens, outcome)
	g.t.genOpOutput.Record(ctx, u.OutputTokens, outcome)
	// kubedojo sends and receives text only.
	text := metric.WithAttributeSet(attribute.NewSet(append(labels[:len(labels):len(labels)], attribute.String(genaiTokenModality, genaiTokenModalityText))...))
	for _, c := range []struct {
		counter metric.Int64Counter
		n       int64
	}{{g.t.genInput, u.InputTokens}, {g.t.genOutput, u.OutputTokens}, {g.t.genCacheRead, u.CacheReadTokens}, {g.t.genCacheWrite, u.CacheWriteTokens}} {
		if c.n > 0 {
			c.counter.Add(ctx, c.n, text)
		}
	}
	return res, err
}

// contentText prepares text for a span: credentials are redacted, and what
// is beyond room bytes is cut, never inside a character. The text is redacted
// before it is cut, so that no cut can leave half a credential behind that
// the rules no longer recognise.
func contentText(redact func(string) string, s string, room int) (text string, cut bool) {
	s = redact(s)
	if len(s) <= room {
		return s, false
	}
	n := max(room, 0)
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + ContentTruncated, true
}

// textPart is one part of a message: {type: "text", content}.
func textPart(content string) attribute.Value {
	return attribute.MapValue(attribute.String(genaiPartType, genaiPartTypeText), attribute.String(genaiPartContent, content))
}

// message is one message: {role, parts: [text part]}.
func message(role, content string) attribute.Value {
	return attribute.MapValue(attribute.String(genaiMessageRole, role),
		attribute.KeyValue{Key: genaiMessageParts, Value: attribute.SliceValue(textPart(content))})
}

// inputMessages is the conversation as it was sent, in its order. All of it
// together holds at most ContentCap bytes of text. The newest message is
// what a reader looks for first, so the room is given out from the end:
// when a conversation is too long, its oldest messages are the ones cut.
func inputMessages(redact func(string) string, msgs []ai.Message) (value attribute.Value, cut bool) {
	texts := make([]string, len(msgs))
	room := ContentCap
	for i := len(msgs) - 1; i >= 0; i-- {
		text, c := contentText(redact, msgs[i].Content, room)
		texts[i], cut = text, cut || c
		room -= min(room, len(text))
	}
	out := make([]attribute.Value, len(msgs))
	for i, m := range msgs {
		out[i] = message(m.Role, texts[i])
	}
	return attribute.SliceValue(out...), cut
}
