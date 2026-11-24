package anthropic

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"

	"github.com/Mistral-valaise/kubedojo/internal/telemetry/teltest"
)

// A traced call to the Claude API is the one provider-neutral span and the
// HTTP request below it: the SDK's own span stays off, or tokens would be
// counted twice. The API gets no trace context, and the span counts cached
// tokens the way every provider's span does.
func TestTracedCallToTheClaudeAPI(t *testing.T) {
	rec := teltest.New()
	// What the process would use if nothing was said: the SDK's own
	// instrumentation would record here, and these headers would be sent.
	tp, prop := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(rec.Provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	t.Cleanup(func() { otel.SetTracerProvider(tp); otel.SetTextMapPropagator(prop) })

	// A fallback model answers; most of the prompt is read from the cache.
	a := &api{events: join([]string{start("claude-sonnet-5-5", 1200, 300)}, thinkingBlock(0), textBlock(1, "The Deployment ", "has no replicas."), stop("end_turn", 42))}
	adapter := provider(t, a, func(c *Config) { c.HTTPClient.Transport = rec.ModelTransport(c.HTTPClient.Transport) })
	p := rec.InstrumentProvider(adapter, func() bool { return false })

	member, _ := baggage.NewMember("learner", "someone")
	bag, _ := baggage.New(member)
	ctx, request := rec.Tracer().Start(baggage.ContextWithBaggage(context.Background(), bag), "request")
	var text strings.Builder
	_, err := p.Chat(ctx, conversation, func(s string) error { text.WriteString(s); return nil })
	request.End()
	if err != nil || text.String() != "The Deployment has no replicas." {
		t.Fatalf("chat: %v, %q", err, text.String())
	}

	want := "request\n   chat claude-opus-5-5\n      POST " + adapter.cfg.BaseURL.Host + "\n"
	if got := teltest.Shape(rec.Ended()); got != want {
		t.Fatalf("the trace:\n%s\nwant:\n%s", got, want)
	}
	chat := rec.Named("chat claude-opus-5-5")[0]
	at := teltest.Attrs(chat)
	for k, v := range map[string]string{"gen_ai.provider.name": "anthropic", "gen_ai.request.model": "claude-opus-5-5",
		"gen_ai.response.model": "claude-sonnet-5-5", "gen_ai.response.id": "msg_test_1", "gen_ai.conversation.id": "s1/q1"} {
		if at[k].AsString() != v {
			t.Errorf("%s = %q, want %q", k, at[k].AsString(), v)
		}
	}
	// input_tokens 25 + cache read 300 + cache write 1200.
	for k, v := range map[string]int64{"gen_ai.usage.input_tokens": 1525, "gen_ai.usage.cache_read.input_tokens": 300,
		"gen_ai.usage.cache_write.input_tokens": 1200, "gen_ai.usage.output_tokens": 42, "gen_ai.request.max_tokens": DefaultMaxTokens} {
		if at[k].AsInt64() != v {
			t.Errorf("%s = %d, want %d", k, at[k].AsInt64(), v)
		}
	}
	if got := at["gen_ai.response.finish_reasons"].AsStringSlice(); len(got) != 1 || got[0] != "stop" {
		t.Errorf("finish reasons %v", got)
	}

	reqs := a.reqs()
	if len(reqs) != 1 {
		t.Fatalf("%d requests", len(reqs))
	}
	for _, name := range []string{"Traceparent", "Tracestate", "Baggage"} {
		if v := reqs[0].header.Get(name); v != "" {
			t.Errorf("the API was sent %s: %s", name, v)
		}
	}
	if all := teltest.Tree(rec.Ended()); strings.Contains(all, testKey) || strings.Contains(all, "THINKING-MUST-NOT-APPEAR") || strings.Contains(all, "second question") {
		t.Errorf("the key, the thinking or the prompt is in the trace:\n%s", all)
	}
}
