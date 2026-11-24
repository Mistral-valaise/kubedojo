package openaicompat

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/ai"
	"github.com/Mistral-valaise/kubedojo/internal/ai/aitest"
	"github.com/Mistral-valaise/kubedojo/internal/ai/netguard"
)

const testKey = "sk-test-key-that-must-never-leak"

// serve starts the fake server and returns a provider for it. change adjusts
// the provider's configuration.
func serve(t *testing.T, fake *aitest.Fake, change func(*Config)) *Provider {
	t.Helper()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	base, err := netguard.CheckURL(srv.URL + "/v1")
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{BaseURL: base, Model: "fake", HTTPClient: netguard.Policy{}.Client(base, netguard.ClientOptions{})}
	if change != nil {
		change(&cfg)
	}
	p, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

var request = ai.Request{System: "You are a test.", Messages: []ai.Message{
	{Role: "user", Content: "first"}, {Role: "assistant", Content: "answer"}, {Role: "user", Content: "<context>\nstate\n</context>\n\nWhy does it fail?"}}}

// chat runs one call and returns the text, the result and the error.
func chat(t *testing.T, p *Provider, ctx context.Context) (string, ai.Result, error) {
	t.Helper()
	var b strings.Builder
	res, err := p.Chat(ctx, request, func(s string) error { b.WriteString(s); return nil })
	return b.String(), res, err
}

func kindOf(err error) ai.ErrorKind {
	var e *ai.Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return ""
}

func TestChatStreams(t *testing.T) {
	fake := &aitest.Fake{RequireKey: testKey}
	p := serve(t, fake, func(c *Config) { c.APIKey = testKey })
	got, res, err := chat(t, p, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := aitest.CannedAnswer("Why does it fail?")
	if got != want {
		t.Errorf("text:\n got %q\nwant %q", got, want)
	}
	if strings.Contains(got, "REASONING") {
		t.Error("the reasoning field ended up in the answer")
	}
	if res.FinishReason != "stop" || res.Model != "fake" || res.ID != "chatcmpl-fake" {
		t.Errorf("result = %+v", res)
	}
	if res.Usage.InputTokens == 0 || res.Usage.OutputTokens == 0 || res.Usage.CacheReadTokens != 3 {
		t.Errorf("usage of the last chunk was not read: %+v", res.Usage)
	}

	reqs := fake.Requests()
	if len(reqs) != 1 {
		t.Fatalf("%d requests", len(reqs))
	}
	r := reqs[0]
	if r.Path != "/v1/chat/completions" || r.Authorization != "Bearer "+testKey {
		t.Errorf("path %q, authorization set = %v", r.Path, r.Authorization != "")
	}
	if r.Body["model"] != "fake" || r.Body["stream"] != true || r.Body["max_tokens"] != float64(DefaultMaxTokens) {
		t.Errorf("body = %v", r.Body)
	}
	if so, _ := r.Body["stream_options"].(map[string]any); so["include_usage"] != true {
		t.Errorf("stream_options = %v", r.Body["stream_options"])
	}
	msgs, _ := r.Body["messages"].([]any)
	var roles []string
	for _, m := range msgs {
		roles = append(roles, m.(map[string]any)["role"].(string))
	}
	if strings.Join(roles, ",") != "system,user,assistant,user" {
		t.Errorf("roles = %v", roles)
	}
}

func TestNoKeyNoAuthorizationHeader(t *testing.T) {
	fake := &aitest.Fake{}
	if _, _, err := chat(t, serve(t, fake, nil), context.Background()); err != nil {
		t.Fatal(err)
	}
	if a := fake.Requests()[0].Authorization; a != "" {
		t.Errorf("Authorization = %q without a key", a)
	}
}

// The stream arrives in arbitrary pieces: in the middle of an event, of a
// line, of a UTF-8 sequence.
func TestChunkBoundariesInsideEvents(t *testing.T) {
	for _, n := range []int{1, 3, 7, 64} {
		got, _, err := chat(t, serve(t, &aitest.Fake{SplitBytes: n}, nil), context.Background())
		if err != nil {
			t.Fatalf("pieces of %d bytes: %v", n, err)
		}
		if want := aitest.CannedAnswer("Why does it fail?"); got != want {
			t.Errorf("pieces of %d bytes:\n got %q\nwant %q", n, got, want)
		}
	}
}

func TestStreamVariants(t *testing.T) {
	for name, fake := range map[string]*aitest.Fake{
		"no usage chunk":       {NoUsage: true},
		"no [DONE]":            {NoDone: true},
		"one JSON document":    {PlainJSON: true},
		"no usage and no done": {NoUsage: true, NoDone: true},
	} {
		got, res, err := chat(t, serve(t, fake, nil), context.Background())
		if err != nil || got != aitest.CannedAnswer("Why does it fail?") || res.FinishReason != "stop" {
			t.Errorf("%s: err=%v finish=%q text=%q", name, err, res.FinishReason, got)
		}
		if name == "one JSON document" && res.Usage.OutputTokens == 0 {
			t.Errorf("%s: usage not read", name)
		}
	}
}

// An upstream error becomes a kind and a message of our own. The body of the
// answer is never passed on: not in the message, not in the error text.
func TestStatusMapping(t *testing.T) {
	for status, want := range map[int]ai.ErrorKind{
		401: ai.ErrAuth, 403: ai.ErrAuth, 404: ai.ErrModelNotFound, 429: ai.ErrRateLimited,
		400: ai.ErrBadRequest, 500: ai.ErrUpstream, 503: ai.ErrUpstream,
	} {
		fake := &aitest.Fake{Status: status}
		got, _, err := chat(t, serve(t, fake, func(c *Config) { c.APIKey = testKey }), context.Background())
		var e *ai.Error
		if !errors.As(err, &e) || e.Kind != want || e.Status != status {
			t.Errorf("HTTP %d: err = %v, want kind %s", status, err, want)
			continue
		}
		if got != "" {
			t.Errorf("HTTP %d: text %q", status, got)
		}
		for _, s := range []string{e.Message, e.Error()} {
			if strings.Contains(s, aitest.BodyMarker) || strings.Contains(s, testKey) {
				t.Errorf("HTTP %d: the error quotes the upstream body or the key: %s", status, s)
			}
		}
		// No retries beyond the one 400 rule.
		if n := len(fake.Requests()); n != 1 {
			t.Errorf("HTTP %d: %d requests, want 1", status, n)
		}
	}
	// The wrong key: the server quotes it back, we do not.
	_, _, err := chat(t, serve(t, &aitest.Fake{RequireKey: "the-right-key"}, func(c *Config) { c.APIKey = testKey }), context.Background())
	if kindOf(err) != ai.ErrAuth || err.(*ai.Error).Message != "The provider rejected the API key." || strings.Contains(err.Error(), testKey) {
		t.Errorf("wrong key: %v", err)
	}
	// An unknown model.
	_, _, err = chat(t, serve(t, &aitest.Fake{Models: []string{"llama3.2"}}, nil), context.Background())
	if kindOf(err) != ai.ErrModelNotFound || !strings.Contains(err.(*ai.Error).Message, `"fake"`) {
		t.Errorf("unknown model: %v", err)
	}
}

func TestRetryOnceOn400(t *testing.T) {
	// max_tokens is not accepted: retry with max_completion_tokens.
	fake := &aitest.Fake{RejectMaxTokens: true}
	got, _, err := chat(t, serve(t, fake, nil), context.Background())
	if err != nil || got == "" {
		t.Fatalf("max_completion_tokens rule: %v", err)
	}
	reqs := fake.Requests()
	if len(reqs) != 2 || reqs[1].Body["max_tokens"] != nil || reqs[1].Body["max_completion_tokens"] != float64(DefaultMaxTokens) {
		t.Errorf("second request = %v", reqs[len(reqs)-1].Body)
	}
	if reqs[1].Body["stream_options"] == nil {
		t.Error("stream_options was dropped although nobody objected to it")
	}

	// stream_options is not accepted: retry without it.
	fake = &aitest.Fake{RejectStreamOptions: true}
	got, res, err := chat(t, serve(t, fake, nil), context.Background())
	if err != nil || got == "" {
		t.Fatalf("stream_options rule: %v", err)
	}
	reqs = fake.Requests()
	if len(reqs) != 2 || reqs[1].Body["stream_options"] != nil || reqs[1].Body["max_tokens"] == nil {
		t.Errorf("second request = %v", reqs[len(reqs)-1].Body)
	}
	if res.Usage != (ai.Usage{}) {
		t.Errorf("usage without stream_options: %+v", res.Usage)
	}

	// Both objections need two retries. There is one.
	fake = &aitest.Fake{RejectMaxTokens: true, RejectStreamOptions: true}
	_, _, err = chat(t, serve(t, fake, nil), context.Background())
	if kindOf(err) != ai.ErrBadRequest || len(fake.Requests()) != 2 {
		t.Errorf("two objections: err=%v after %d requests", err, len(fake.Requests()))
	}

	// A 400 about something else is not retried.
	fake = &aitest.Fake{Status: 400}
	_, _, err = chat(t, serve(t, fake, nil), context.Background())
	if kindOf(err) != ai.ErrBadRequest || len(fake.Requests()) != 1 {
		t.Errorf("other 400: err=%v after %d requests", err, len(fake.Requests()))
	}
}

func TestRedirectIsNotFollowed(t *testing.T) {
	elsewhere := &aitest.Fake{}
	other := httptest.NewServer(elsewhere)
	defer other.Close()
	fake := &aitest.Fake{RedirectTo: other.URL + "/v1/chat/completions"}
	_, _, err := chat(t, serve(t, fake, func(c *Config) { c.APIKey = testKey }), context.Background())
	var e *ai.Error
	if !errors.As(err, &e) || e.Kind != ai.ErrUpstream || e.Status != 302 || !strings.Contains(e.Message, "redirect") {
		t.Errorf("err = %v", err)
	}
	if n := len(elsewhere.Requests()); n != 0 {
		t.Errorf("the redirect was followed: the other server got %d requests (and the key)", n)
	}
}

func TestSlowFirstByteTimesOut(t *testing.T) {
	p := serve(t, &aitest.Fake{FirstByteDelay: 2 * time.Second}, func(c *Config) { c.FirstByte = 50 * time.Millisecond })
	start := time.Now()
	_, _, err := chat(t, p, context.Background())
	if kindOf(err) != ai.ErrTimeout {
		t.Errorf("err = %v, want a timeout", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("gave up after %s", d)
	}
}

func TestIdleStreamTimesOut(t *testing.T) {
	p := serve(t, &aitest.Fake{StallAfter: 3}, func(c *Config) { c.Idle = 80 * time.Millisecond })
	got, _, err := chat(t, p, context.Background())
	if kindOf(err) != ai.ErrTimeout {
		t.Errorf("err = %v, want a timeout", err)
	}
	if got == "" {
		t.Error("the text that arrived before the stall is lost")
	}
}

func TestTotalTimeLimit(t *testing.T) {
	p := serve(t, &aitest.Fake{ChunkDelay: 20 * time.Millisecond}, func(c *Config) { c.Total = 100 * time.Millisecond })
	_, _, err := chat(t, p, context.Background())
	if kindOf(err) != ai.ErrTimeout {
		t.Errorf("err = %v, want a timeout", err)
	}
}

func TestOutputCap(t *testing.T) {
	long := strings.Repeat("wörd ", 2000) // 12000 bytes
	p := serve(t, &aitest.Fake{Answer: func(string) string { return long }}, func(c *Config) { c.MaxOutputBytes = 998 })
	got, res, err := chat(t, p, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// The cap falls inside the two bytes of an "ö": the text ends before it.
	if len(got) != 997 {
		t.Errorf("%d bytes, want 997 (the cap is 998)", len(got))
	}
	if !strings.HasPrefix(long, got) {
		t.Error("the capped text is not a prefix of the answer (cut inside a character?)")
	}
	if res.FinishReason != "length" {
		t.Errorf("finish reason = %q, want length", res.FinishReason)
	}
}

func TestCancellation(t *testing.T) {
	fake := &aitest.Fake{ChunkDelay: 20 * time.Millisecond}
	p := serve(t, fake, nil)
	ctx, cancel := context.WithCancel(context.Background())
	n := 0
	_, err := p.Chat(ctx, request, func(string) error {
		if n++; n == 3 {
			cancel()
		}
		return nil
	})
	if kindOf(err) != ai.ErrCanceled {
		t.Errorf("err = %v, want canceled", err)
	}
	// A listener that fails ends the call as well.
	_, err = p.Chat(context.Background(), request, func(string) error { return errors.New("gone") })
	if kindOf(err) != ai.ErrCanceled {
		t.Errorf("failing onDelta: err = %v, want canceled", err)
	}
}

func TestBrokenStreams(t *testing.T) {
	_, _, err := chat(t, serve(t, &aitest.Fake{Garbage: true}, nil), context.Background())
	if kindOf(err) != ai.ErrUpstream {
		t.Errorf("garbage: %v", err)
	}
	_, _, err = chat(t, serve(t, &aitest.Fake{ErrorEvent: true}, nil), context.Background())
	if kindOf(err) != ai.ErrUpstream || strings.Contains(err.Error(), aitest.BodyMarker) {
		t.Errorf("error event: %v", err)
	}
}

func TestUnreachable(t *testing.T) {
	// A port nothing listens on.
	srv := httptest.NewServer(http.NotFoundHandler())
	addr := srv.URL
	srv.Close()
	base, _ := url.Parse(addr + "/v1")
	p, err := New(Config{BaseURL: base, Model: "m", HTTPClient: netguard.Policy{}.Client(base, netguard.ClientOptions{})})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Chat(context.Background(), request, func(string) error { return nil })
	var e *ai.Error
	if !errors.As(err, &e) || e.Kind != ai.ErrUnreachable || !strings.HasPrefix(e.Message, "Could not reach 127.0.0.1") {
		t.Errorf("err = %v", err)
	}

	// The Ollama port on this machine gets the Ollama hint. The guard refuses
	// the dial here so that the test does not depend on what runs on 11434.
	base, _ = url.Parse("http://localhost:11434/v1")
	e = ai.FromTransport(context.Background(), &netOpError, base)
	if e.Kind != ai.ErrUnreachable || e.Message != "Could not reach http://localhost:11434. Is Ollama running?" {
		t.Errorf("ollama hint: %+v", e)
	}
}

var netOpError = net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}

func TestBlockedURL(t *testing.T) {
	// The policy refuses when the connection is made.
	base, _ := url.Parse("http://169.254.169.254/v1")
	p, err := New(Config{BaseURL: base, Model: "m", HTTPClient: netguard.Policy{}.Client(base, netguard.ClientOptions{})})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Chat(context.Background(), request, func(string) error { return nil })
	if kindOf(err) != ai.ErrBlockedURL {
		t.Errorf("err = %v, want blocked_url", err)
	}
}

func TestNameAndServer(t *testing.T) {
	for raw, want := range map[string]string{
		"https://api.openai.com/v1":         "openai",
		"http://localhost:11434/v1":         "ollama",
		"http://gpu.lan:11434/v1":           "ollama",
		"https://openrouter.ai/api/v1":      "openrouter",
		"https://api.groq.com/openai/v1":    "groq",
		"https://api.mistral.ai/v1":         "mistral_ai",
		"https://api.deepseek.com":          "deepseek",
		"https://api.x.ai/v1":               "x_ai",
		"http://localhost:1234/v1":          "openai_compatible",
		"https://notapi.openai.com.evil/v1": "openai_compatible",
	} {
		base, _ := url.Parse(raw)
		if got := NameFor(base); got != want {
			t.Errorf("NameFor(%s) = %q, want %q", raw, got, want)
		}
	}
	for raw, want := range map[string]int{"https://api.openai.com/v1": 443, "http://gpu.lan/v1": 80, "http://localhost:11434/v1": 11434} {
		base, _ := url.Parse(raw)
		p, err := New(Config{BaseURL: base, Model: "m", HTTPClient: http.DefaultClient})
		if err != nil {
			t.Fatal(err)
		}
		if host, port := p.Server(); port != want || host != base.Hostname() {
			t.Errorf("Server(%s) = %s:%d", raw, host, port)
		}
	}
}

func TestNewRequiresModelAndBaseURL(t *testing.T) {
	base, _ := url.Parse("http://localhost:11434/v1")
	if _, err := New(Config{BaseURL: base, HTTPClient: http.DefaultClient}); kindOf(err) != ai.ErrNotConfigured {
		t.Errorf("no model: %v", err)
	}
	if _, err := New(Config{Model: "m", HTTPClient: http.DefaultClient}); kindOf(err) != ai.ErrNotConfigured {
		t.Errorf("no base URL: %v", err)
	}
}
