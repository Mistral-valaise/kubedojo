package telemetry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/Mistral-valaise/kubedojo/internal/ai"
	"github.com/Mistral-valaise/kubedojo/internal/ai/aitest"
	"github.com/Mistral-valaise/kubedojo/internal/ai/openaicompat"
	"github.com/Mistral-valaise/kubedojo/internal/checker"
	"github.com/Mistral-valaise/kubedojo/internal/span"
	"github.com/Mistral-valaise/kubedojo/internal/telemetry"
	"github.com/Mistral-valaise/kubedojo/internal/telemetry/teltest"
)

// scripted is a provider that answers as it is told.
type scripted struct {
	name, model string
	host        string
	port        int
	limit       int // what MaxOutputTokens reports
	wait        time.Duration
	deltas      []string
	res         ai.Result
	err         error
	errAfter    int
}

func (s *scripted) Name() string          { return s.name }
func (s *scripted) Model() string         { return s.model }
func (s *scripted) Server() (string, int) { return s.host, s.port }

func (s *scripted) MaxOutputTokens(req ai.Request) int {
	if req.MaxOutputTokens > 0 {
		return req.MaxOutputTokens
	}
	return s.limit
}

func (s *scripted) Chat(ctx context.Context, req ai.Request, onDelta func(string) error) (ai.Result, error) {
	time.Sleep(s.wait)
	for i, d := range s.deltas {
		if s.err != nil && i == s.errAfter {
			return s.res, s.err
		}
		if err := onDelta(d); err != nil {
			return s.res, &ai.Error{Kind: ai.ErrCanceled, Message: "The request was cancelled.", Err: err}
		}
	}
	if s.err != nil && s.errAfter >= len(s.deltas) {
		return s.res, s.err
	}
	return s.res, nil
}

func discard(string) error { return nil }

func on() bool  { return true }
func off() bool { return false }

// recorder is a Telemetry that records spans and redacts content as the
// server does.
func recorder() *teltest.Recorder {
	rec := teltest.New()
	rec.Redact = checker.Redact
	return rec
}

// recorderWith is recorder with a meter provider and a logger.
func recorderWith(mp metric.MeterProvider, logger *slog.Logger) *teltest.Recorder {
	rec := teltest.NewWith(mp, logger)
	rec.Redact = checker.Redact
	return rec
}

// one returns the only ended span whose name starts with "chat ".
func chatSpan(t *testing.T, rec *teltest.Recorder) sdktrace.ReadOnlySpan {
	t.Helper()
	var found []sdktrace.ReadOnlySpan
	for _, s := range rec.Ended() {
		if strings.HasPrefix(s.Name(), "chat ") {
			found = append(found, s)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d chat spans:\n%s", len(found), teltest.Tree(rec.Ended()))
	}
	return found[0]
}

func keys(m map[string]attribute.Value) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// The span of a call to a model, attribute by attribute, as the GenAI
// conventions name them (semantic-conventions-genai v0.1.0).
func TestChatSpanFollowsTheConventions(t *testing.T) {
	rec := recorder()
	p := rec.InstrumentProvider(&scripted{name: "anthropic", model: "claude-opus-5-5", host: "api.anthropic.com", port: 443, limit: 16000,
		wait: 15 * time.Millisecond, deltas: []string{"The Deployment ", "has no replicas."},
		// A fallback model answered, and most of the prompt came from the cache.
		res: ai.Result{ID: "msg_01", Model: "claude-sonnet-5-5", FinishReason: "stop",
			Usage: ai.Usage{InputTokens: 1525, OutputTokens: 42, CacheReadTokens: 300, CacheWriteTokens: 1200}}}, off)
	if p.Name() != "anthropic" || p.Model() != "claude-opus-5-5" {
		t.Fatalf("the wrapped provider is %s/%s", p.Name(), p.Model())
	}
	ctx, parent := rec.Tracer().Start(context.Background(), "assistant.chat")
	var got strings.Builder
	res, err := p.Chat(ctx, ai.Request{System: "SYSTEM", ConversationID: "s1/q1", Messages: []ai.Message{{Role: "user", Content: "why?"}}},
		func(s string) error { got.WriteString(s); return nil })
	parent.End()
	if err != nil || res.ID != "msg_01" || got.String() != "The Deployment has no replicas." {
		t.Fatalf("the call is not passed through: %+v, %v, %q", res, err, got.String())
	}

	s := chatSpan(t, rec)
	if s.Name() != "chat claude-opus-5-5" {
		t.Errorf("name %q, want \"{gen_ai.operation.name} {gen_ai.request.model}\"", s.Name())
	}
	if s.SpanKind() != trace.SpanKindClient {
		t.Errorf("kind %v, want client", s.SpanKind())
	}
	if s.Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Error("the span is not a child of the span of its context")
	}
	if s.Status().Code != codes.Unset {
		t.Errorf("status %v of a call that worked", s.Status())
	}
	a := teltest.Attrs(s)
	want := map[string]any{
		"gen_ai.operation.name":                 "chat",
		"gen_ai.provider.name":                  "anthropic",
		"gen_ai.request.model":                  "claude-opus-5-5",
		"gen_ai.request.stream":                 true,
		"gen_ai.request.max_tokens":             int64(16000),
		"gen_ai.conversation.id":                "s1/q1",
		"server.address":                        "api.anthropic.com",
		"server.port":                           int64(443),
		"gen_ai.response.id":                    "msg_01",
		"gen_ai.response.model":                 "claude-sonnet-5-5",
		"gen_ai.response.finish_reasons":        []string{"stop"},
		"gen_ai.usage.input_tokens":             int64(1525),
		"gen_ai.usage.output_tokens":            int64(42),
		"gen_ai.usage.cache_read.input_tokens":  int64(300),
		"gen_ai.usage.cache_write.input_tokens": int64(1200),
	}
	for k, v := range want {
		if got, ok := a[k]; !ok || fmt.Sprint(got.AsInterface()) != fmt.Sprint(v) {
			t.Errorf("%s = %v, want %v", k, got.AsInterface(), v)
		}
	}
	// The time to the first chunk: after the wait, before the end.
	first := a["gen_ai.response.time_to_first_chunk"].AsFloat64()
	if total := s.EndTime().Sub(s.StartTime()).Seconds(); first < 0.015 || first > total {
		t.Errorf("gen_ai.response.time_to_first_chunk = %v s of a call of %v s that waited 15 ms", first, total)
	}
	delete(a, "gen_ai.response.time_to_first_chunk")
	// Nothing else: no content, no error, nothing nobody decided on.
	if len(a) != len(want) {
		t.Errorf("attributes: %v\nwant: the %d above and the time to the first chunk", keys(a), len(want))
	}
	if len(s.Events()) != 0 {
		t.Errorf("events: %v", s.Events())
	}
}

// What is optional is left out when it is not known, not recorded as zero.
func TestChatSpanLeavesOutWhatIsNotKnown(t *testing.T) {
	rec := recorder()
	p := rec.InstrumentProvider(&scripted{name: "ollama", model: "llama3.2", deltas: []string{"ok"}, res: ai.Result{FinishReason: "stop", Model: "llama3.2"}}, off)
	if _, err := p.Chat(context.Background(), ai.Request{}, discard); err != nil {
		t.Fatal(err)
	}
	a := teltest.Attrs(chatSpan(t, rec))
	for _, k := range []string{"gen_ai.conversation.id", "gen_ai.request.max_tokens", "server.address", "server.port", "gen_ai.response.id",
		"gen_ai.usage.input_tokens", "gen_ai.usage.output_tokens", "gen_ai.usage.cache_read.input_tokens", "gen_ai.usage.cache_write.input_tokens", "error.type"} {
		if _, ok := a[k]; ok {
			t.Errorf("%s = %v on a call that did not say", k, a[k].AsInterface())
		}
	}
}

// Every kind of failure: a fault is an error of the span and names its kind.
// A caller that left and a model that declined are no faults of the server.
func TestChatSpanErrorMapping(t *testing.T) {
	for _, kind := range []ai.ErrorKind{ai.ErrNotConfigured, ai.ErrAuth, ai.ErrModelNotFound, ai.ErrRateLimited, ai.ErrUnreachable,
		ai.ErrTimeout, ai.ErrBadRequest, ai.ErrUpstream, ai.ErrBlockedURL, ai.ErrRefused, ai.ErrCanceled} {
		t.Run(string(kind), func(t *testing.T) {
			reader := sdkmetric.NewManualReader()
			rec := recorderWith(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)), slog.New(slog.NewTextHandler(io.Discard, nil)))
			cause := errors.New("CAUSE-MARKER: upstream said things")
			p := rec.InstrumentProvider(&scripted{name: "openai", model: "m", host: "api.openai.com", port: 443,
				err: &ai.Error{Kind: kind, Status: 418, Message: "A message for the learner.", Err: cause}}, on)
			_, err := p.Chat(context.Background(), ai.Request{Messages: []ai.Message{{Role: "user", Content: "q"}}}, discard)
			if ai.AsError(err).Kind != kind {
				t.Fatalf("the error is not passed through: %v", err)
			}
			s := chatSpan(t, rec)
			a := teltest.Attrs(s)
			fault := kind != ai.ErrRefused && kind != ai.ErrCanceled
			if fault {
				if s.Status().Code != codes.Error || a["error.type"].AsString() != string(kind) {
					t.Errorf("status %v, error.type %q; want Error and %s", s.Status().Code, a["error.type"].AsString(), kind)
				}
				if s.Status().Description != "A message for the learner." {
					t.Errorf("status description %q", s.Status().Description)
				}
			} else {
				if _, has := a["error.type"]; s.Status().Code != codes.Unset || has {
					t.Errorf("status %v, error.type %v; want Unset and none", s.Status().Code, a["error.type"].AsInterface())
				}
			}
			switch kind {
			case ai.ErrCanceled:
				if a["kubedojo.refused"].AsString() != "cancelled" {
					t.Errorf("kubedojo.refused = %q", a["kubedojo.refused"].AsString())
				}
			case ai.ErrRefused:
				if got := a["gen_ai.response.finish_reasons"].AsStringSlice(); len(got) != 1 || got[0] != "content_filter" {
					t.Errorf("finish reasons %v, want content_filter", got)
				}
			default:
				// Nothing was received, so there is no finish reason and
				// no model that answered.
				if _, has := a["gen_ai.response.finish_reasons"]; has {
					t.Errorf("finish reasons %v of a call that got no answer", a["gen_ai.response.finish_reasons"].AsInterface())
				}
			}
			if _, has := a["gen_ai.response.model"]; has {
				t.Error("gen_ai.response.model of a call nothing answered")
			}
			// The cause can quote the upstream: it stays in the log.
			if strings.Contains(teltest.Tree(rec.Ended()), "CAUSE-MARKER") {
				t.Errorf("the cause of the error is in the trace:\n%s", teltest.Tree(rec.Ended()))
			}
			// The metrics agree with the span.
			for _, name := range []string{"gen_ai.client.inference.duration", "gen_ai.client.inference.operation.input_tokens", "gen_ai.client.inference.operation.output_tokens"} {
				pts := points(t, reader, name)
				if len(pts) != 1 {
					t.Fatalf("%s: %d data points", name, len(pts))
				}
				got, has := pts[0].attrs.Value("error.type")
				if fault && got.AsString() != string(kind) || !fault && has {
					t.Errorf("%s: error.type = %q, fault = %v", name, got.AsString(), fault)
				}
			}
			if pts := points(t, reader, "gen_ai.client.inference.time_to_first_chunk"); len(pts) != 0 {
				t.Errorf("a time to the first chunk without a chunk: %v", pts)
			}
		})
	}
}

// A call that fails after text arrived: the generation ended in an error,
// and what did arrive counts.
func TestChatSpanOfACallThatFailsHalfWay(t *testing.T) {
	rec := recorder()
	p := rec.InstrumentProvider(&scripted{name: "openai", model: "gpt", deltas: []string{"The first half", " never comes"}, errAfter: 1,
		res: ai.Result{ID: "chatcmpl-1", Model: "gpt-2026-01-01", Usage: ai.Usage{InputTokens: 80}},
		err: &ai.Error{Kind: ai.ErrUpstream, Message: "The connection was interrupted."}}, on)
	if _, err := p.Chat(context.Background(), ai.Request{Messages: []ai.Message{{Role: "user", Content: "q"}}}, discard); err == nil {
		t.Fatal("no error")
	}
	s := chatSpan(t, rec)
	a := teltest.Attrs(s)
	if got := a["gen_ai.response.finish_reasons"].AsStringSlice(); len(got) != 1 || got[0] != "error" {
		t.Errorf("finish reasons %v, want error", got)
	}
	if a["gen_ai.response.model"].AsString() != "gpt-2026-01-01" || a["gen_ai.usage.input_tokens"].AsInt64() != 80 {
		t.Errorf("what the answer said before it broke off is missing: %v", keys(a))
	}
	if out := a["gen_ai.output.messages"].String(); !strings.Contains(out, "The first half") || strings.Contains(out, "never comes") {
		t.Errorf("the partial output: %s", out)
	}
}

// The caller leaves while the answer streams: not an error of anybody.
func TestChatSpanWhenTheCallerLeaves(t *testing.T) {
	rec := recorder()
	p := rec.InstrumentProvider(&aitest.Provider{Deltas: []string{"one ", "two ", "three"}}, off)
	ctx, cancel := context.WithCancel(context.Background())
	_, err := p.Chat(ctx, ai.Request{}, func(s string) error {
		cancel()
		return errors.New("nobody listens")
	})
	if ai.AsError(err).Kind != ai.ErrCanceled {
		t.Fatalf("err = %v", err)
	}
	s := chatSpan(t, rec)
	a := teltest.Attrs(s)
	if s.Status().Code != codes.Unset || a["kubedojo.refused"].AsString() != "cancelled" {
		t.Errorf("status %v, kubedojo.refused %q", s.Status().Code, a["kubedojo.refused"].AsString())
	}
	if _, has := a["error.type"]; has {
		t.Error("error.type on a cancelled call")
	}
	if got := a["gen_ai.response.finish_reasons"].AsStringSlice(); len(got) != 1 || got[0] != "error" {
		t.Errorf("finish reasons %v: the generation did not end properly", got)
	}
}

const marker = "PROMPT-MARKER-7f3a"

func markedRequest() ai.Request {
	return ai.Request{System: "You are a tutor. " + marker + "-system", ConversationID: "s1/q1", Messages: []ai.Message{
		{Role: "user", Content: "first question " + marker + "-old"},
		{Role: "assistant", Content: "first answer " + marker + "-reply"},
		{Role: "user", Content: "<context>\npassword: hunter2-" + marker + "\n</context>\n\nsecond question " + marker + "-new"},
	}}
}

// everything is every string a traced call left behind: names, attributes,
// status, events and links of every span.
func everything(rec *teltest.Recorder) string {
	var b strings.Builder
	for _, s := range rec.Ended() {
		fmt.Fprintln(&b, s.Name(), s.Status().Description)
		for _, kv := range s.Attributes() {
			fmt.Fprintln(&b, kv.Key, kv.Value.String())
		}
		for _, e := range s.Events() {
			fmt.Fprintln(&b, e.Name)
			for _, kv := range e.Attributes {
				fmt.Fprintln(&b, kv.Key, kv.Value.String())
			}
		}
		for _, l := range s.Links() {
			for _, kv := range l.Attributes {
				fmt.Fprintln(&b, kv.Key, kv.Value.String())
			}
		}
		for _, kv := range s.Resource().Attributes() {
			fmt.Fprintln(&b, kv.Key, kv.Value.String())
		}
	}
	return b.String()
}

// Without the opt-in a trace holds nothing of the prompt and nothing of the
// answer: not an attribute, not an event.
func TestContentIsNotRecordedByDefault(t *testing.T) {
	for name, content := range map[string]func() bool{"off": off, "nil": nil} {
		t.Run(name, func(t *testing.T) {
			rec := recorder()
			p := rec.InstrumentProvider(&scripted{name: "ollama", model: "llama3.2", deltas: []string{"An answer with ", marker, "-answer."},
				res: ai.Result{ID: "c1", Model: "llama3.2", FinishReason: "stop", Usage: ai.Usage{InputTokens: 10, OutputTokens: 5}}}, content)
			if _, err := p.Chat(context.Background(), markedRequest(), discard); err != nil {
				t.Fatal(err)
			}
			all := everything(rec)
			if strings.Contains(all, marker) || strings.Contains(all, "hunter2") {
				t.Errorf("text of the conversation is in the trace:\n%s", all)
			}
			a := teltest.Attrs(chatSpan(t, rec))
			for _, k := range []string{"gen_ai.system_instructions", "gen_ai.input.messages", "gen_ai.output.messages", "kubedojo.assistant.content.truncated"} {
				if _, ok := a[k]; ok {
					t.Errorf("%s exists without the opt-in", k)
				}
			}
			// What is recorded either way.
			if a["gen_ai.usage.output_tokens"].AsInt64() != 5 || a["gen_ai.conversation.id"].AsString() != "s1/q1" {
				t.Errorf("token counts and the conversation are missing: %v", keys(a))
			}
		})
	}
}

// part returns the text of the only part of a recorded message, and its role.
func part(t *testing.T, msg attribute.Value) (role, content string) {
	t.Helper()
	if msg.Type() != attribute.MAP {
		t.Fatalf("a message is recorded as %v, want a map", msg.Type())
	}
	var parts []attribute.Value
	for _, kv := range msg.AsMap() {
		switch string(kv.Key) {
		case "role":
			role = kv.Value.AsString()
		case "parts":
			parts = kv.Value.AsSlice()
		default:
			t.Errorf("a message has the field %q", kv.Key)
		}
	}
	if len(parts) != 1 {
		t.Fatalf("a message has %d parts", len(parts))
	}
	return role, textOf(t, parts[0])
}

// textOf returns the content of a recorded text part.
func textOf(t *testing.T, p attribute.Value) string {
	t.Helper()
	fields := map[string]string{}
	for _, kv := range p.AsMap() {
		fields[string(kv.Key)] = kv.Value.AsString()
	}
	if p.Type() != attribute.MAP || fields["type"] != "text" || len(fields) != 2 {
		t.Fatalf("a part is %v, want {type: text, content}", p.String())
	}
	return fields["content"]
}

// With the opt-in: the system instructions, the messages in their order and
// the answer, as structured values in the shape the conventions define, with
// credentials redacted.
func TestContentIsRecordedWhenSwitchedOn(t *testing.T) {
	rec := recorder()
	p := rec.InstrumentProvider(&scripted{name: "ollama", model: "llama3.2",
		deltas: []string{"Look at the Secret: ", "token: eyJhbGciOiJSUzI1NiIsImtpZCI6IiJ9.eyJpc3MiOiJrdWJlcm5ldGVzIn0.c2lnbmF0dXJl ", marker, "-answer"},
		res:    ai.Result{ID: "c1", Model: "llama3.2", FinishReason: "stop"}}, on)
	if _, err := p.Chat(context.Background(), markedRequest(), discard); err != nil {
		t.Fatal(err)
	}
	a := teltest.Attrs(chatSpan(t, rec))

	sys := a["gen_ai.system_instructions"]
	if sys.Type() != attribute.SLICE || len(sys.AsSlice()) != 1 || textOf(t, sys.AsSlice()[0]) != "You are a tutor. "+marker+"-system" {
		t.Errorf("gen_ai.system_instructions = %s", sys.String())
	}

	in := a["gen_ai.input.messages"]
	if in.Type() != attribute.SLICE || len(in.AsSlice()) != 3 {
		t.Fatalf("gen_ai.input.messages = %s", in.String())
	}
	for i, want := range []struct{ role, has string }{{"user", marker + "-old"}, {"assistant", marker + "-reply"}, {"user", "second question " + marker + "-new"}} {
		role, content := part(t, in.AsSlice()[i])
		if role != want.role || !strings.Contains(content, want.has) {
			t.Errorf("input message %d: %s %q", i, role, content)
		}
	}
	// The password of the context block is gone, and only that.
	if _, last := part(t, in.AsSlice()[2]); strings.Contains(last, "hunter2") || !strings.Contains(last, "password: [redacted]") {
		t.Errorf("the newest message is not redacted: %q", last)
	}

	out := a["gen_ai.output.messages"]
	if out.Type() != attribute.SLICE || len(out.AsSlice()) != 1 {
		t.Fatalf("gen_ai.output.messages = %s", out.String())
	}
	role, answer := part(t, out.AsSlice()[0])
	if role != "assistant" || !strings.HasSuffix(answer, marker+"-answer") || strings.Contains(answer, "eyJhbGci") {
		t.Errorf("output message: %s %q", role, answer)
	}
	if _, cut := a["kubedojo.assistant.content.truncated"]; cut {
		t.Error("kubedojo.assistant.content.truncated on content that fits")
	}
	// The opt-in is asked per call: off for the next one.
	rec2 := recorder()
	enabled := true
	p2 := rec2.InstrumentProvider(&scripted{name: "ollama", model: "m", deltas: []string{"x"}}, func() bool { return enabled })
	enabled = false
	if _, err := p2.Chat(context.Background(), markedRequest(), discard); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(everything(rec2), marker) {
		t.Error("content was recorded although the opt-in was withdrawn before the call")
	}
}

// Recording never changes the answer: the caller gets every piece, also of
// an answer that is far larger than what a span keeps of it.
func TestRecordingContentDoesNotTouchTheAnswer(t *testing.T) {
	rec := recorder()
	piece := strings.Repeat("ö", 4<<10) // 8 KiB
	var deltas []string
	for i := 0; i < 48; i++ { // 384 KiB: more than the buffer for the span
		deltas = append(deltas, piece)
	}
	p := rec.InstrumentProvider(&scripted{name: "ollama", model: "m", deltas: deltas}, on)
	var got strings.Builder
	if _, err := p.Chat(context.Background(), ai.Request{}, func(s string) error { got.WriteString(s); return nil }); err != nil {
		t.Fatal(err)
	}
	if got.String() != strings.Join(deltas, "") {
		t.Errorf("the caller got %d bytes of an answer of %d", got.Len(), len(deltas)*len(piece))
	}
	a := teltest.Attrs(chatSpan(t, rec))
	_, out := part(t, a["gen_ai.output.messages"].AsSlice()[0])
	if len(out) > telemetry.ContentCap+len(telemetry.ContentTruncated) || !strings.HasSuffix(out, telemetry.ContentTruncated) || !a["kubedojo.assistant.content.truncated"].AsBool() {
		t.Errorf("the span keeps %d bytes of the answer", len(out))
	}
}

// A Telemetry nobody gave a redaction records no content, opt-in or not:
// text never reaches a trace unredacted.
func TestContentIsNotRecordedWithoutARedaction(t *testing.T) {
	rec := teltest.New()
	p := rec.InstrumentProvider(&scripted{name: "ollama", model: "m", deltas: []string{marker, "-answer"}}, on)
	if _, err := p.Chat(context.Background(), markedRequest(), discard); err != nil {
		t.Fatal(err)
	}
	if all := everything(rec); strings.Contains(all, marker) || strings.Contains(all, "hunter2") {
		t.Errorf("content was recorded without a redaction:\n%s", all)
	}
}

// Each content attribute holds at most 32 KiB of text. Of a conversation
// that is longer the oldest messages are the ones cut; nothing is dropped,
// and the span says that it was cut.
func TestContentIsCapped(t *testing.T) {
	rec := recorder()
	long := func(tag string, n int) string { return tag + strings.Repeat("ä", n/2) + tag }
	req := ai.Request{System: long("SYS", 40<<10), Messages: []ai.Message{
		{Role: "user", Content: long("OLDEST", 20<<10)},
		{Role: "assistant", Content: long("MIDDLE", 20<<10)},
		{Role: "user", Content: long("NEWEST", 20<<10)},
	}}
	var deltas []string
	for i := 0; i < 50; i++ {
		deltas = append(deltas, strings.Repeat("ö", 512))
	}
	p := rec.InstrumentProvider(&scripted{name: "ollama", model: "m", deltas: deltas}, on)
	if _, err := p.Chat(context.Background(), req, discard); err != nil {
		t.Fatal(err)
	}
	a := teltest.Attrs(chatSpan(t, rec))
	if !a["kubedojo.assistant.content.truncated"].AsBool() {
		t.Error("kubedojo.assistant.content.truncated is not set")
	}
	const cap, mark = telemetry.ContentCap, telemetry.ContentTruncated
	if cap != 32<<10 {
		t.Errorf("the cap is %d bytes", cap)
	}
	valid := func(what, s string) {
		t.Helper()
		if !strings.HasSuffix(s, mark) || len(s) > cap+len(mark) || strings.ContainsRune(s, '�') {
			t.Errorf("%s: %d bytes, ends in %q", what, len(s), s[max(0, len(s)-20):])
		}
	}
	sys := textOf(t, a["gen_ai.system_instructions"].AsSlice()[0])
	valid("system instructions", sys)
	if !strings.HasPrefix(sys, "SYS") {
		t.Errorf("system instructions start with %q", sys[:10])
	}

	msgs := a["gen_ai.input.messages"].AsSlice()
	if len(msgs) != 3 {
		t.Fatalf("%d input messages, want all 3", len(msgs))
	}
	_, oldest := part(t, msgs[0])
	_, middle := part(t, msgs[1])
	_, newest := part(t, msgs[2])
	if newest != req.Messages[2].Content {
		t.Errorf("the newest message was cut: %d of %d bytes", len(newest), len(req.Messages[2].Content))
	}
	valid("the message before", middle)
	if !strings.HasPrefix(middle, "MIDDLE") || oldest != mark {
		t.Errorf("middle starts with %q, oldest is %q", middle[:10], oldest)
	}
	if total := len(oldest) + len(middle) + len(newest); total > cap+2*len(mark) {
		t.Errorf("the input messages hold %d bytes", total)
	}

	_, out := part(t, a["gen_ai.output.messages"].AsSlice()[0])
	valid("output", out)
}

type point struct {
	attrs     attribute.Set
	sum       float64
	count     uint64
	bounds    []float64
	exemplars []trace.TraceID
}

// points returns the data points of a metric, and fails if its unit is not
// the one the conventions give.
func points(t *testing.T, reader *sdkmetric.ManualReader, name string) []point {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	units := map[string]string{
		"gen_ai.client.inference.duration":                       "s",
		"gen_ai.client.inference.time_to_first_chunk":            "s",
		"gen_ai.client.inference.usage.input_tokens":             "{token}",
		"gen_ai.client.inference.usage.output_tokens":            "{token}",
		"gen_ai.client.inference.usage.cache_read.input_tokens":  "{token}",
		"gen_ai.client.inference.usage.cache_write.input_tokens": "{token}",
		"gen_ai.client.inference.operation.input_tokens":         "{token}",
		"gen_ai.client.inference.operation.output_tokens":        "{token}",
		"kubedojo.assistant.requests":                            "",
		"kubedojo.assistant.context.duration":                    "s",
	}
	var out []point
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			if want, ok := units[name]; !ok || m.Unit != want {
				t.Errorf("%s has the unit %q, want %q", name, m.Unit, want)
			}
			switch d := m.Data.(type) {
			case metricdata.Histogram[float64]:
				for _, p := range d.DataPoints {
					pt := point{attrs: p.Attributes, sum: p.Sum, count: p.Count, bounds: p.Bounds}
					for _, e := range p.Exemplars {
						pt.exemplars = append(pt.exemplars, trace.TraceID(e.TraceID))
					}
					out = append(out, pt)
				}
			case metricdata.Histogram[int64]:
				for _, p := range d.DataPoints {
					pt := point{attrs: p.Attributes, sum: float64(p.Sum), count: p.Count, bounds: p.Bounds}
					for _, e := range p.Exemplars {
						pt.exemplars = append(pt.exemplars, trace.TraceID(e.TraceID))
					}
					out = append(out, pt)
				}
			case metricdata.Sum[int64]:
				if !d.IsMonotonic {
					t.Errorf("%s is not a counter", name)
				}
				for _, p := range d.DataPoints {
					pt := point{attrs: p.Attributes, sum: float64(p.Value), count: 1}
					for _, e := range p.Exemplars {
						pt.exemplars = append(pt.exemplars, trace.TraceID(e.TraceID))
					}
					out = append(out, pt)
				}
			default:
				t.Fatalf("%s has data %T", name, m.Data)
			}
		}
	}
	return out
}

func labels(set attribute.Set) string {
	var parts []string
	for _, kv := range set.ToSlice() {
		parts = append(parts, string(kv.Key)+"="+kv.Value.String())
	}
	return strings.Join(parts, " ")
}

// The client metrics of the conventions: names, units, bucket boundaries,
// labels, and the trace of the call as an exemplar on every sample.
func TestGenAIMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	rec := recorderWith(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)), slog.New(slog.NewTextHandler(io.Discard, nil)))
	p := rec.InstrumentProvider(&scripted{name: "anthropic", model: "claude-opus-5-5", host: "api.anthropic.com", port: 443,
		wait: 5 * time.Millisecond, deltas: []string{"a", "b"},
		res: ai.Result{ID: "msg_01", Model: "claude-sonnet-5-5", FinishReason: "stop",
			Usage: ai.Usage{InputTokens: 1525, OutputTokens: 42, CacheReadTokens: 300, CacheWriteTokens: 1200}}}, on)
	ctx, request := rec.Tracer().Start(context.Background(), "request")
	if _, err := p.Chat(ctx, markedRequest(), discard); err != nil {
		t.Fatal(err)
	}
	request.End()
	traceID := request.SpanContext().TraceID()

	const base = "gen_ai.operation.name=chat gen_ai.provider.name=anthropic gen_ai.request.model=claude-opus-5-5 gen_ai.response.model=claude-sonnet-5-5"
	const server = " server.address=api.anthropic.com server.port=443"
	seconds := []float64{0.01, 0.02, 0.04, 0.08, 0.16, 0.32, 0.64, 1.28, 2.56, 5.12, 10.24, 20.48, 40.96, 81.92}
	tokens := []float64{1, 4, 16, 64, 256, 1024, 4096, 16384, 65536, 262144, 1048576, 4194304, 16777216, 67108864}
	for _, c := range []struct {
		name   string
		labels string
		sum    float64 // -1: a duration, only checked to be positive
		bounds []float64
	}{
		{"gen_ai.client.inference.duration", base + server, -1, seconds},
		{"gen_ai.client.inference.time_to_first_chunk", base + server, -1, seconds},
		{"gen_ai.client.inference.operation.input_tokens", base + server, 1525, tokens},
		{"gen_ai.client.inference.operation.output_tokens", base + server, 42, tokens},
		{"gen_ai.client.inference.usage.input_tokens", base + " gen_ai.token.modality=text" + server, 1525, nil},
		{"gen_ai.client.inference.usage.output_tokens", base + " gen_ai.token.modality=text" + server, 42, nil},
		{"gen_ai.client.inference.usage.cache_read.input_tokens", base + " gen_ai.token.modality=text" + server, 300, nil},
		{"gen_ai.client.inference.usage.cache_write.input_tokens", base + " gen_ai.token.modality=text" + server, 1200, nil},
	} {
		pts := points(t, reader, c.name)
		if len(pts) != 1 {
			t.Errorf("%s: %d data points, want 1", c.name, len(pts))
			continue
		}
		pt := pts[0]
		if got := labels(pt.attrs); got != c.labels {
			t.Errorf("%s is labelled\n  %s\nwant\n  %s", c.name, got, c.labels)
		}
		if c.sum >= 0 && pt.sum != c.sum || c.sum < 0 && pt.sum <= 0 || pt.count != 1 {
			t.Errorf("%s: sum %v, count %d", c.name, pt.sum, pt.count)
		}
		if fmt.Sprint(pt.bounds) != fmt.Sprint(c.bounds) {
			t.Errorf("%s has the buckets %v, want %v", c.name, pt.bounds, c.bounds)
		}
		if len(pt.exemplars) != 1 || pt.exemplars[0] != traceID {
			t.Errorf("%s: exemplars %v, want the trace %s of the call", c.name, pt.exemplars, traceID)
		}
		// Nothing of the conversation becomes a label.
		if l := labels(pt.attrs); strings.Contains(l, "conversation") || strings.Contains(l, "s1/q1") || strings.Contains(l, marker) {
			t.Errorf("%s is labelled with the conversation: %s", c.name, l)
		}
	}
	first, total := points(t, reader, "gen_ai.client.inference.time_to_first_chunk")[0].sum, points(t, reader, "gen_ai.client.inference.duration")[0].sum
	if first < 0.005 || first > total {
		t.Errorf("first chunk after %v s of %v s", first, total)
	}
}

// A provider that reports no usage: the per-call histograms record 0, the
// counters nothing.
func TestGenAIMetricsWithoutUsage(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	rec := recorderWith(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)), slog.New(slog.NewTextHandler(io.Discard, nil)))
	p := rec.InstrumentProvider(&scripted{name: "ollama", model: "m", deltas: []string{"x"}, res: ai.Result{Model: "m", FinishReason: "stop"}}, off)
	if _, err := p.Chat(context.Background(), ai.Request{}, discard); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"gen_ai.client.inference.operation.input_tokens", "gen_ai.client.inference.operation.output_tokens"} {
		if pts := points(t, reader, name); len(pts) != 1 || pts[0].count != 1 || pts[0].sum != 0 {
			t.Errorf("%s: %+v, want one sample of 0", name, pts)
		}
	}
	for _, name := range []string{"gen_ai.client.inference.usage.input_tokens", "gen_ai.client.inference.usage.output_tokens",
		"gen_ai.client.inference.usage.cache_read.input_tokens", "gen_ai.client.inference.usage.cache_write.input_tokens"} {
		if pts := points(t, reader, name); len(pts) != 0 {
			t.Errorf("%s: %+v, want nothing", name, pts)
		}
	}
}

// kubedojo's own metrics of the assistant.
func TestAssistantMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	rec := recorderWith(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)), slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, sp := rec.Tracer().Start(context.Background(), "assistant.chat")
	rec.AssistantRequest(ctx, "ollama", "answer", "ok", "cka", "practice")
	rec.AssistantContext(ctx, "cka", false, 1500*time.Millisecond)
	sp.End()
	rec.AssistantRequest(context.Background(), "none", "admission", "not_configured", "kcna", "practice")

	reqs := points(t, reader, "kubedojo.assistant.requests")
	got := map[string]bool{}
	for _, p := range reqs {
		got[labels(p.attrs)] = true
	}
	for _, want := range []string{"mode=practice phase=answer provider=ollama result=ok track=cka", "mode=practice phase=admission provider=none result=not_configured track=kcna"} {
		if !got[want] {
			t.Errorf("kubedojo.assistant.requests lacks {%s}: %v", want, got)
		}
	}
	gather := points(t, reader, "kubedojo.assistant.context.duration")
	if len(gather) != 1 || labels(gather[0].attrs) != "cached=false track=cka" || gather[0].sum != 1.5 {
		t.Fatalf("kubedojo.assistant.context.duration: %+v", gather)
	}
	if len(gather[0].exemplars) != 1 || gather[0].exemplars[0] != sp.SpanContext().TraceID() {
		t.Errorf("exemplars of the context duration: %v", gather[0].exemplars)
	}
}

// headerRecorder is a model server that keeps the headers it was sent.
type headerRecorder struct {
	next http.Handler
	mu   sync.Mutex
	seen []http.Header
}

func (h *headerRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	h.seen = append(h.seen, r.Header.Clone())
	h.mu.Unlock()
	h.next.ServeHTTP(w, r)
}

func (h *headerRecorder) headers() []http.Header {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]http.Header(nil), h.seen...)
}

const keyMarker = "sk-KEY-MARKER-0123456789abcdef"

// A model provider is a third party. The HTTP request to it is a span of our
// trace, and nothing of that trace travels with it, whatever the process
// propagates elsewhere. The key it is sent stays out of everything recorded.
func TestModelCallsCarryNoTraceContext(t *testing.T) {
	// The process propagates trace context and baggage, as the server does.
	before := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	t.Cleanup(func() { otel.SetTextMapPropagator(before) })

	fake := &headerRecorder{next: &aitest.Fake{RequireKey: keyMarker}}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	base, _ := url.Parse(srv.URL + "/v1")

	var logs bytes.Buffer
	reader := sdkmetric.NewManualReader()
	rec := recorderWith(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)), slog.New(slog.NewTextHandler(&logs, nil)))
	inner := &http.Transport{}
	t.Cleanup(inner.CloseIdleConnections)
	adapter, err := openaicompat.New(openaicompat.Config{BaseURL: base, APIKey: keyMarker, Model: "fake",
		HTTPClient: &http.Client{Transport: rec.ModelTransport(inner)}})
	if err != nil {
		t.Fatal(err)
	}
	p := rec.InstrumentProvider(adapter, on)

	member, _ := baggage.NewMember("learner", "someone")
	bag, _ := baggage.New(member)
	ctx, request := rec.Tracer().Start(baggage.ContextWithBaggage(context.Background(), bag), "request")
	var answer strings.Builder
	res, err := p.Chat(ctx, markedRequest(), func(s string) error { answer.WriteString(s); return nil })
	request.End()
	if err != nil || !strings.Contains(answer.String(), "Fake model") {
		t.Fatalf("chat: %v, %q", err, answer.String())
	}

	got := fake.headers()
	if len(got) != 1 {
		t.Fatalf("%d requests reached the model server", len(got))
	}
	for _, name := range []string{"Traceparent", "Tracestate", "Baggage"} {
		if v := got[0].Get(name); v != "" {
			t.Errorf("the model server was sent %s: %s", name, v)
		}
	}
	if got[0].Get("Authorization") != "Bearer "+keyMarker {
		t.Fatalf("the request was not authorised: %v", got[0])
	}

	// The HTTP span is below the chat span, named after method and host.
	chat := chatSpan(t, rec)
	kids := rec.Children(chat)
	if len(kids) != 1 || kids[0].Name() != "POST "+base.Host || kids[0].SpanKind() != trace.SpanKindClient {
		t.Fatalf("below the chat span:\n%s", teltest.Tree(rec.Ended()))
	}
	if code := teltest.Attrs(kids[0])["http.response.status_code"].AsInt64(); code != 200 {
		t.Errorf("http.response.status_code = %d", code)
	}
	a := teltest.Attrs(chat)
	if a["gen_ai.provider.name"].AsString() != "openai_compatible" || a["server.address"].AsString() != "127.0.0.1" ||
		a["gen_ai.request.max_tokens"].AsInt64() != openaicompat.DefaultMaxTokens || a["gen_ai.response.id"].AsString() != "chatcmpl-fake" {
		t.Errorf("the chat span of the real adapter: %v", teltest.Tree([]sdktrace.ReadOnlySpan{chat}))
	}
	// The server counts cached tokens within prompt_tokens, as ai.Usage does.
	if a["gen_ai.usage.input_tokens"].AsInt64() != res.Usage.InputTokens || a["gen_ai.usage.cache_read.input_tokens"].AsInt64() != 3 || res.Usage.InputTokens <= 3 {
		t.Errorf("usage: %+v on the span %v", res.Usage, a["gen_ai.usage.input_tokens"].AsInterface())
	}

	// The key is nowhere: not in a span, a metric label or a log line.
	if all := everything(rec); strings.Contains(all, keyMarker) || strings.Contains(all, "KEY-MARKER") {
		t.Errorf("the key is in the trace:\n%s", all)
	}
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	dump, err := json.Marshal(rm)
	if err != nil || !bytes.Contains(dump, []byte("openai_compatible")) {
		t.Fatalf("the metrics cannot be searched: %v", err)
	}
	if bytes.Contains(dump, []byte("KEY-MARKER")) || bytes.Contains(dump, []byte(marker)) {
		t.Errorf("the key or the prompt is in the metrics: %s", dump)
	}
	if strings.Contains(logs.String(), "KEY-MARKER") {
		t.Errorf("the key is in the log: %s", logs.String())
	}
	// The HTTP instrumentation records no metrics of its own.
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if strings.HasPrefix(m.Name, "http.client") {
				t.Errorf("uncatalogued metric %s", m.Name)
			}
		}
	}
}

// A key the server rejects: the failed call names its kind and nothing of
// the key or of the body the server answered with.
func TestARejectedKeyStaysOutOfTheTrace(t *testing.T) {
	srv := httptest.NewServer(&aitest.Fake{RequireKey: "the-right-key"})
	t.Cleanup(srv.Close)
	base, _ := url.Parse(srv.URL + "/v1")
	rec := recorder()
	inner := &http.Transport{}
	t.Cleanup(inner.CloseIdleConnections)
	adapter, err := openaicompat.New(openaicompat.Config{BaseURL: base, APIKey: keyMarker, Model: "fake",
		HTTPClient: &http.Client{Transport: rec.ModelTransport(inner)}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = rec.InstrumentProvider(adapter, on).Chat(context.Background(), markedRequest(), discard)
	if ai.AsError(err).Kind != ai.ErrAuth {
		t.Fatalf("err = %v", err)
	}
	chat := chatSpan(t, rec)
	if a := teltest.Attrs(chat); a["error.type"].AsString() != "auth" || chat.Status().Code != codes.Error {
		t.Errorf("the chat span: %s", teltest.Tree(rec.Ended()))
	}
	// The server quotes the key in its error body.
	if all := everything(rec); strings.Contains(all, "KEY-MARKER") || strings.Contains(all, aitest.BodyMarker) {
		t.Errorf("the key or the upstream body is in the trace:\n%s", all)
	}
}

// The trace of a context, for code that does not know OpenTelemetry.
func TestTraceID(t *testing.T) {
	rec := recorder()
	ctx, sp := rec.Start(context.Background(), "work")
	id := span.TraceID(ctx)
	sp.End(nil)
	ended := rec.Named("work")
	if len(ended) != 1 || id != ended[0].SpanContext().TraceID().String() || len(id) != 32 {
		t.Errorf("span.TraceID = %q, the span is in %v", id, ended)
	}
	// No span, no tracer, or a span that has ended: no id to hand out.
	if got := span.TraceID(span.With(context.Background(), rec)); got != "" {
		t.Errorf("a context without a span has the trace %q", got)
	}
	if got := span.TraceID(context.Background()); got != "" {
		t.Errorf("a context without a tracer has the trace %q", got)
	}
	if got := span.TraceID(ctx); got != "" {
		t.Errorf("an ended span still hands out its trace %q", got)
	}
}
