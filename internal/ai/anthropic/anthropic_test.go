package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/ai"
	"github.com/Mistral-valaise/kubedojo/internal/ai/netguard"
)

const (
	testKey    = "sk-ant-test-key-that-must-never-leak"
	bodyMarker = "UPSTREAM-BODY-MARKER"
)

// api is a fake Claude API: it records the requests and answers each with
// the events (or the error) the test set.
type api struct {
	mu       sync.Mutex
	requests []recorded
	events   []string // SSE data payloads, in order
	status   int      // when set: an error answer
	delay    time.Duration
}

type recorded struct {
	path   string
	header http.Header
	body   map[string]any
}

func (a *api) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	a.mu.Lock()
	a.requests = append(a.requests, recorded{r.URL.Path + "?" + r.URL.RawQuery, r.Header.Clone(), body})
	a.mu.Unlock()
	if a.status != 0 {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Request-Id", "req_test_1")
		w.Header().Set("Retry-After-Ms", "1")
		w.WriteHeader(a.status)
		fmt.Fprintf(w, `{"type":"error","error":{"type":"some_error","message":"%s for key %s"},"request_id":"req_test_1"}`, bodyMarker, r.Header.Get("Authorization"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	fl := w.(http.Flusher)
	for _, data := range a.events {
		var ev struct{ Type string }
		_ = json.Unmarshal([]byte(data), &ev)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data)
		fl.Flush()
		if a.delay > 0 {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(a.delay):
			}
		}
	}
}

func (a *api) reqs() []recorded {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]recorded(nil), a.requests...)
}

func start(model string, cacheWrite, cacheRead int) string {
	return fmt.Sprintf(`{"type":"message_start","message":{"id":"msg_test_1","type":"message","role":"assistant","model":%q,"content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":25,"cache_creation_input_tokens":%d,"cache_read_input_tokens":%d,"output_tokens":1}}}`, model, cacheWrite, cacheRead)
}

func textBlock(index int, parts ...string) []string {
	out := []string{fmt.Sprintf(`{"type":"content_block_start","index":%d,"content_block":{"type":"text","text":""}}`, index)}
	for _, p := range parts {
		out = append(out, fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"text_delta","text":%q}}`, index, p))
	}
	return append(out, fmt.Sprintf(`{"type":"content_block_stop","index":%d}`, index))
}

func thinkingBlock(index int) []string {
	return []string{
		fmt.Sprintf(`{"type":"content_block_start","index":%d,"content_block":{"type":"thinking","thinking":"","signature":""}}`, index),
		fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"thinking_delta","thinking":"THINKING-MUST-NOT-APPEAR"}}`, index),
		fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"signature_delta","signature":"c2ln"}}`, index),
		fmt.Sprintf(`{"type":"content_block_stop","index":%d}`, index),
	}
}

func stop(reason string, outputTokens int) []string {
	return []string{
		fmt.Sprintf(`{"type":"message_delta","delta":{"stop_reason":%q,"stop_sequence":null},"usage":{"output_tokens":%d}}`, reason, outputTokens),
		`{"type":"message_stop"}`,
	}
}

func join(parts ...[]string) []string {
	var out []string
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func provider(t *testing.T, a *api, change func(*Config)) *Provider {
	t.Helper()
	srv := httptest.NewServer(a)
	t.Cleanup(srv.Close)
	base, err := netguard.CheckURL(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{APIKey: testKey, BaseURL: base, Model: DefaultModel, HTTPClient: netguard.Policy{}.Client(base, netguard.ClientOptions{})}
	if change != nil {
		change(&cfg)
	}
	p, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

var conversation = ai.Request{System: "SYSTEM PROMPT", ConversationID: "s1/q1", Messages: []ai.Message{
	{Role: "user", Content: "first question"},
	{Role: "assistant", Content: "first answer"},
	{Role: "user", Content: "<context>\nstate\n</context>\n\nsecond question"},
}}

func chat(t *testing.T, p *Provider, ctx context.Context) (string, ai.Result, error) {
	t.Helper()
	var b strings.Builder
	res, err := p.Chat(ctx, conversation, func(s string) error { b.WriteString(s); return nil })
	return b.String(), res, err
}

func TestChatStreamsTextAndFillsTheResult(t *testing.T) {
	a := &api{events: join([]string{start("claude-opus-5-5", 1200, 300)}, thinkingBlock(0), textBlock(1, "The Deployment ", "has no ", "replicas."), stop("end_turn", 42))}
	got, res, err := chat(t, provider(t, a, nil), context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "The Deployment has no replicas." {
		t.Errorf("text = %q", got)
	}
	// The API reports 25 input tokens next to 300 read from and 1200
	// written to the cache. ai.Usage counts all of them as input.
	want := ai.Result{ID: "msg_test_1", Model: "claude-opus-5-5", FinishReason: "stop",
		Usage: ai.Usage{InputTokens: 1525, OutputTokens: 42, CacheReadTokens: 300, CacheWriteTokens: 1200}}
	if res != want {
		t.Errorf("result:\n got %+v\nwant %+v", res, want)
	}
}

// What is sent: the parameters the tutor always used, the system prompt as a
// cached block, and the history as plain text turns.
func TestRequest(t *testing.T) {
	a := &api{events: join([]string{start("claude-opus-5-5", 0, 0)}, textBlock(0, "ok"), stop("end_turn", 1))}
	if _, _, err := chat(t, provider(t, a, nil), context.Background()); err != nil {
		t.Fatal(err)
	}
	reqs := a.reqs()
	if len(reqs) != 1 {
		t.Fatalf("%d requests", len(reqs))
	}
	r := reqs[0]
	if !strings.HasPrefix(r.path, "/v1/messages") {
		t.Errorf("path = %s", r.path)
	}
	// SDK v2 sends an API key as a bearer token, like an auth token.
	if r.header.Get("Authorization") != "Bearer "+testKey {
		t.Errorf("the key is not sent")
	}
	if beta := strings.Join(r.header.Values("Anthropic-Beta"), ","); !strings.Contains(beta, "server-side-fallback-2026-07-01") {
		t.Errorf("anthropic-beta = %q", beta)
	}
	if r.header.Get("Traceparent") != "" {
		t.Error("the SDK's own OpenTelemetry instrumentation is on")
	}
	b := r.body
	if b["model"] != "claude-opus-5-5" || b["max_tokens"] != float64(16000) || b["stream"] != true || b["fallbacks"] != "default" {
		t.Errorf("model/max_tokens/stream/fallbacks = %v %v %v %v", b["model"], b["max_tokens"], b["stream"], b["fallbacks"])
	}
	if oc, _ := b["output_config"].(map[string]any); oc["effort"] != "medium" {
		t.Errorf("output_config = %v", b["output_config"])
	}
	for _, absent := range []string{"thinking", "temperature", "tools", "tool_choice"} {
		if _, ok := b[absent]; ok {
			t.Errorf("the request sets %s", absent)
		}
	}
	system, _ := b["system"].([]any)
	if len(system) != 1 {
		t.Fatalf("system = %v", b["system"])
	}
	sys := system[0].(map[string]any)
	if cc, _ := sys["cache_control"].(map[string]any); sys["text"] != "SYSTEM PROMPT" || cc["type"] != "ephemeral" {
		t.Errorf("system block = %v", sys)
	}
	msgs, _ := b["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("%d messages", len(msgs))
	}
	for i, want := range []struct{ role, text string }{
		{"user", "first question"}, {"assistant", "first answer"}, {"user", "<context>\nstate\n</context>\n\nsecond question"}} {
		m := msgs[i].(map[string]any)
		content, _ := m["content"].([]any)
		if m["role"] != want.role || len(content) != 1 {
			t.Errorf("message %d = %v", i, m)
			continue
		}
		block := content[0].(map[string]any)
		if block["type"] != "text" || block["text"] != want.text {
			t.Errorf("message %d block = %v", i, block)
		}
	}
}

func TestRequestVariants(t *testing.T) {
	// A model without the fallback beta does not send it.
	a := &api{events: join([]string{start("claude-haiku-5-5", 0, 0)}, textBlock(0, "ok"), stop("end_turn", 1))}
	if _, _, err := chat(t, provider(t, a, func(c *Config) { c.Model = "claude-haiku-5-5" }), context.Background()); err != nil {
		t.Fatal(err)
	}
	r := a.reqs()[0]
	if _, ok := r.body["fallbacks"]; ok || strings.Contains(strings.Join(r.header.Values("Anthropic-Beta"), ","), "fallback") {
		t.Error("the fallback beta is sent for a model that does not have it")
	}

	// An auth token (ANTHROPIC_AUTH_TOKEN) is sent as a bearer token.
	a = &api{events: join([]string{start("claude-opus-5-5", 0, 0)}, textBlock(0, "ok"), stop("end_turn", 1))}
	if _, _, err := chat(t, provider(t, a, func(c *Config) { c.AuthToken = true }), context.Background()); err != nil {
		t.Fatal(err)
	}
	r = a.reqs()[0]
	if r.header.Get("Authorization") != "Bearer "+testKey || r.header.Get("X-Api-Key") != "" {
		t.Error("the auth token is not sent as a bearer token")
	}

	// A small output limit of the request is passed on (the settings test).
	a = &api{events: join([]string{start("claude-opus-5-5", 0, 0)}, stop("max_tokens", 8))}
	p := provider(t, a, nil)
	res, err := p.Chat(context.Background(), ai.Request{Messages: []ai.Message{{Role: "user", Content: "ping"}}, MaxOutputTokens: 8}, func(string) error { return nil })
	if err != nil || res.FinishReason != "length" {
		t.Errorf("max_tokens stop: %v %+v", err, res)
	}
	r = a.reqs()[0]
	if r.body["max_tokens"] != float64(8) || r.body["system"] != nil {
		t.Errorf("max_tokens = %v, system = %v", r.body["max_tokens"], r.body["system"])
	}
}

// The environment must not configure the client behind kubedojo's back.
func TestEnvironmentIsNotRead(t *testing.T) {
	t.Setenv("ANTHROPIC_BASE_URL", "http://127.0.0.1:1")
	t.Setenv("ANTHROPIC_API_KEY", "key-from-the-environment")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "token-from-the-environment")
	t.Setenv("ANTHROPIC_CUSTOM_HEADERS", "X-From-Env: yes")
	a := &api{events: join([]string{start("claude-opus-5-5", 0, 0)}, textBlock(0, "ok"), stop("end_turn", 1))}
	if _, _, err := chat(t, provider(t, a, nil), context.Background()); err != nil {
		t.Fatalf("the request did not reach the configured server: %v", err)
	}
	h := a.reqs()[0].header
	if h.Get("Authorization") != "Bearer "+testKey || h.Get("X-Api-Key") != "" || h.Get("X-From-Env") != "" {
		t.Errorf("the environment leaked into the request: configured key sent = %v, x-api-key set = %v, custom header = %q",
			h.Get("Authorization") == "Bearer "+testKey, h.Get("X-Api-Key") != "", h.Get("X-From-Env"))
	}
}

func TestRefusal(t *testing.T) {
	// Declined before any output.
	a := &api{events: []string{start("claude-opus-5-5", 0, 0),
		`{"type":"message_delta","delta":{"stop_reason":"refusal","stop_sequence":null,"stop_details":{"type":"refusal","category":"cyber","explanation":"declined"}},"usage":{"output_tokens":0}}`,
		`{"type":"message_stop"}`}}
	got, _, err := chat(t, provider(t, a, nil), context.Background())
	var e *ai.Error
	if !errors.As(err, &e) || e.Kind != ai.ErrRefused || got != "" {
		t.Errorf("refusal: err=%v text=%q", err, got)
	}

	// Declined in the middle: the text so far was streamed, and the call
	// still ends as a refusal.
	a = &api{events: join([]string{start("claude-opus-5-5", 0, 0)}, textBlock(0, "Partial "), stop("refusal", 3))}
	got, _, err = chat(t, provider(t, a, nil), context.Background())
	if !errors.As(err, &e) || e.Kind != ai.ErrRefused || got != "Partial " {
		t.Errorf("mid-stream refusal: err=%v text=%q", err, got)
	}
}

// A fallback model takes over on the same stream. The text of both models is
// the answer, and the result names the model that finished it.
func TestServerSideFallbackOnTheStream(t *testing.T) {
	a := &api{events: join(
		[]string{start("claude-opus-5-5", 0, 0)},
		textBlock(0, "Start, "),
		[]string{
			`{"type":"content_block_start","index":1,"content_block":{"type":"fallback","from":{"model":"claude-opus-5-5"},"to":{"model":"claude-opus-4-8"},"trigger":{"type":"refusal","category":null}}}`,
			`{"type":"content_block_stop","index":1}`,
		},
		textBlock(2, "continued."),
		stop("end_turn", 9))}
	got, res, err := chat(t, provider(t, a, nil), context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "Start, continued." || res.Model != "claude-opus-4-8" || res.FinishReason != "stop" {
		t.Errorf("text=%q model=%q finish=%q", got, res.Model, res.FinishReason)
	}
}

func TestErrorMapping(t *testing.T) {
	for status, want := range map[int]struct {
		kind     ai.ErrorKind
		requests int // 1, or 2 where the SDK retries once
	}{
		401: {ai.ErrAuth, 1}, 403: {ai.ErrAuth, 1}, 404: {ai.ErrModelNotFound, 1}, 400: {ai.ErrBadRequest, 1},
		429: {ai.ErrRateLimited, 2}, 500: {ai.ErrUpstream, 2}, 529: {ai.ErrRateLimited, 2},
	} {
		a := &api{status: status}
		got, _, err := chat(t, provider(t, a, nil), context.Background())
		var e *ai.Error
		if !errors.As(err, &e) || e.Kind != want.kind || e.Status != status || got != "" {
			t.Errorf("HTTP %d: err = %v, want kind %s", status, err, want.kind)
			continue
		}
		for _, s := range []string{e.Message, e.Error()} {
			if strings.Contains(s, bodyMarker) || strings.Contains(s, testKey) {
				t.Errorf("HTTP %d: the error quotes the upstream body or the key: %s", status, s)
			}
		}
		if !strings.Contains(e.Error(), "req_test_1") {
			t.Errorf("HTTP %d: the request id is not in the log text: %s", status, e.Error())
		}
		if n := len(a.reqs()); n != want.requests {
			t.Errorf("HTTP %d: %d requests, want %d", status, n, want.requests)
		}
	}
}

func TestCancelTimeoutAndUnreachable(t *testing.T) {
	slow := func() *api {
		return &api{delay: 30 * time.Millisecond, events: join([]string{start("claude-opus-5-5", 0, 0)}, textBlock(0, "a", "b", "c", "d", "e", "f", "g", "h"), stop("end_turn", 8))}
	}
	// The caller goes away.
	ctx, cancel := context.WithCancel(context.Background())
	p := provider(t, slow(), nil)
	n := 0
	_, err := p.Chat(ctx, conversation, func(string) error {
		if n++; n == 2 {
			cancel()
		}
		return nil
	})
	var e *ai.Error
	if !errors.As(err, &e) || e.Kind != ai.ErrCanceled {
		t.Errorf("cancel: %v", err)
	}
	// The listener fails.
	_, err = provider(t, slow(), nil).Chat(context.Background(), conversation, func(string) error { return errors.New("gone") })
	if !errors.As(err, &e) || e.Kind != ai.ErrCanceled {
		t.Errorf("failing onDelta: %v", err)
	}
	// The call takes longer than it may.
	got, _, err := chat(t, provider(t, slow(), func(c *Config) { c.Total = 100 * time.Millisecond }), context.Background())
	if !errors.As(err, &e) || e.Kind != ai.ErrTimeout || got == "" {
		t.Errorf("timeout: err=%v text=%q", err, got)
	}
	// Nothing listens there.
	srv := httptest.NewServer(http.NotFoundHandler())
	base, _ := url.Parse(srv.URL)
	srv.Close()
	p, err = New(Config{APIKey: testKey, BaseURL: base, HTTPClient: netguard.Policy{}.Client(base, netguard.ClientOptions{})})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Chat(context.Background(), conversation, func(string) error { return nil })
	if !errors.As(err, &e) || e.Kind != ai.ErrUnreachable {
		t.Errorf("unreachable: %v", err)
	}
	// The policy refuses the address.
	base, _ = url.Parse("http://169.254.169.254")
	p, _ = New(Config{APIKey: testKey, BaseURL: base, HTTPClient: netguard.Policy{}.Client(base, netguard.ClientOptions{})})
	_, err = p.Chat(context.Background(), conversation, func(string) error { return nil })
	if !errors.As(err, &e) || e.Kind != ai.ErrBlockedURL {
		t.Errorf("blocked: %v", err)
	}
}

func TestNameServerAndConfig(t *testing.T) {
	if _, err := New(Config{HTTPClient: http.DefaultClient}); err == nil {
		t.Error("a provider without a key was built")
	}
	p, err := New(Config{APIKey: testKey, HTTPClient: http.DefaultClient})
	if err != nil {
		t.Fatal(err)
	}
	if host, port := p.Server(); p.Name() != "anthropic" || p.Model() != DefaultModel || host != "api.anthropic.com" || port != 443 {
		t.Errorf("%s %s %s:%d", p.Name(), p.Model(), host, port)
	}
	base, _ := url.Parse("http://localhost:8080")
	p, _ = New(Config{APIKey: testKey, BaseURL: base, HTTPClient: http.DefaultClient})
	if host, port := p.Server(); host != "localhost" || port != 8080 {
		t.Errorf("%s:%d", host, port)
	}
}
