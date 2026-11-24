// Package aitest has stand-ins for a model provider: a fake
// OpenAI-compatible server and a fake ai.Provider. Tests use them, and so
// does anybody who wants to try the assistant without a model:
//
//	go run ./internal/ai/aitest/cmd/fakeopenai -addr 127.0.0.1:4497
//	kubedojo up --ai --ai-provider openai --ai-base-url http://127.0.0.1:4497/v1 --ai-model fake
package aitest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/ai"
)

// BodyMarker is part of every error body the fake server sends. A test that
// finds it in what kubedojo shows or logs has found an upstream body there.
const BodyMarker = "UPSTREAM-BODY-MARKER"

// Request is one request the fake server received.
type Request struct {
	Path          string
	Authorization string
	Body          map[string]any
}

// LastUser is the content of the last user message of the request.
func (r Request) LastUser() string {
	msgs, _ := r.Body["messages"].([]any)
	for i := len(msgs) - 1; i >= 0; i-- {
		if m, ok := msgs[i].(map[string]any); ok && m["role"] == "user" {
			s, _ := m["content"].(string)
			return s
		}
	}
	return ""
}

// Fake is a fake OpenAI-compatible server. The zero value streams a canned
// Markdown answer that echoes the last user message. The fields change what
// it does; set them before the first request.
type Fake struct {
	// Status, when set, is the status of every answer, with an error body.
	Status int
	// RequireKey, when set, answers 401 unless this bearer token is sent.
	RequireKey string
	// Models, when set, are the only models that exist; others get 404.
	Models []string
	// RejectMaxTokens answers 400 to a request with max_tokens and names
	// max_completion_tokens, as newer OpenAI models do.
	RejectMaxTokens bool
	// RejectStreamOptions answers 400 to a request with stream_options.
	RejectStreamOptions bool
	// RedirectTo, when set, answers 302 to this URL.
	RedirectTo string
	// FirstByteDelay is waited before anything is sent.
	FirstByteDelay time.Duration
	// ChunkDelay is waited between two pieces of the stream.
	ChunkDelay time.Duration
	// SplitBytes, when set, writes the stream in pieces of this many bytes,
	// so that the pieces end inside events and inside UTF-8 sequences.
	SplitBytes int
	// Answer makes the answer text from the last user message.
	Answer func(lastUser string) string
	// NoUsage leaves the usage chunk out; NoDone the final "[DONE]".
	NoUsage, NoDone bool
	// PlainJSON answers with one JSON document although a stream was asked for.
	PlainJSON bool
	// StallAfter, when positive, stops sending after this many content
	// events and holds the connection open until the client leaves.
	StallAfter int
	// Garbage sends an event that is not JSON.
	Garbage bool
	// ErrorEvent sends an error object in the stream after the first event.
	ErrorEvent bool

	mu       sync.Mutex
	requests []Request
	gone     int
}

// ClientsGone is how many requests ended because the client went away
// while the server was still answering.
func (f *Fake) ClientsGone() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gone
}

// Requests returns what the server received so far.
func (f *Fake) Requests() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Request(nil), f.requests...)
}

// CannedAnswer is the default answer of the fake server.
func CannedAnswer(lastUser string) string {
	// The question is quoted without the context block kubedojo puts in front.
	if i := strings.LastIndex(lastUser, "</context>"); i >= 0 {
		lastUser = lastUser[i+len("</context>"):]
	}
	return "**Fake model.** You asked:\n\n> " + strings.TrimSpace(lastUser) + "\n\nLook at the failing check first:\n\n```bash\nk get pods -A\n```\n\nNothing here was computed by a model — ünïcödé included."
}

func (f *Fake) fail(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": msg + " " + BodyMarker, "type": "invalid_request_error"}})
}

// ServeHTTP implements POST /v1/chat/completions (and /chat/completions).
func (f *Fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/chat/completions") {
		f.fail(w, http.StatusNotFound, "no such endpoint")
		return
	}
	var body map[string]any
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	_ = json.Unmarshal(raw, &body)
	req := Request{Path: r.URL.Path, Authorization: r.Header.Get("Authorization"), Body: body}
	f.mu.Lock()
	f.requests = append(f.requests, req)
	f.mu.Unlock()

	model, _ := body["model"].(string)
	switch {
	case f.RedirectTo != "":
		http.Redirect(w, r, f.RedirectTo, http.StatusFound)
		return
	case f.RequireKey != "" && req.Authorization != "Bearer "+f.RequireKey:
		f.fail(w, http.StatusUnauthorized, "Incorrect API key provided: "+strings.TrimPrefix(req.Authorization, "Bearer "))
		return
	case f.Status != 0:
		f.fail(w, f.Status, "the fake server was told to fail")
		return
	case len(f.Models) > 0 && !contains(f.Models, model):
		f.fail(w, http.StatusNotFound, fmt.Sprintf("model %q not found, try pulling it first", model))
		return
	case f.RejectMaxTokens && body["max_tokens"] != nil:
		f.fail(w, http.StatusBadRequest, "Unsupported parameter: 'max_tokens' is not supported with this model. Use 'max_completion_tokens' instead.")
		return
	case f.RejectStreamOptions && body["stream_options"] != nil:
		f.fail(w, http.StatusBadRequest, "Unrecognized request argument supplied: stream_options")
		return
	}

	answer := f.Answer
	if answer == nil {
		answer = CannedAnswer
	}
	text := answer(req.LastUser())
	usage := map[string]any{"prompt_tokens": len(raw) / 4, "completion_tokens": len(text) / 4, "total_tokens": (len(raw) + len(text)) / 4,
		"prompt_tokens_details": map[string]any{"cached_tokens": 3}}
	if !sleep(r.Context(), f.FirstByteDelay) {
		return
	}
	if f.PlainJSON {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "chatcmpl-fake", "model": model, "usage": usage,
			"choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": text}}}})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	out := &splitter{w: w, n: f.SplitBytes, ctx: r.Context()}
	defer out.finish()
	defer func() {
		if r.Context().Err() != nil {
			f.mu.Lock()
			f.gone++
			f.mu.Unlock()
		}
	}()
	event := func(v any) bool {
		b, _ := json.Marshal(v)
		return out.write("data: " + string(b) + "\n\n")
	}
	chunk := func(delta map[string]any, finish any) map[string]any {
		return map[string]any{"id": "chatcmpl-fake", "object": "chat.completion.chunk", "model": model,
			"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}}
	}
	if !event(chunk(map[string]any{"role": "assistant", "content": ""}, nil)) {
		return
	}
	if f.Garbage {
		out.write("data: {this is not JSON\n\n")
	}
	if f.ErrorEvent {
		event(map[string]any{"error": map[string]any{"message": "the model crashed " + BodyMarker}})
		return
	}
	// A reasoning model sends its thoughts in another field. They are not
	// part of the answer.
	if !event(chunk(map[string]any{"reasoning_content": "REASONING-MUST-NOT-APPEAR", "content": nil}, nil)) {
		return
	}
	for i, piece := range pieces(text) {
		if f.StallAfter > 0 && i == f.StallAfter {
			out.finish()
			<-r.Context().Done()
			return
		}
		if !event(chunk(map[string]any{"content": piece}, nil)) {
			return
		}
		if !sleep(r.Context(), f.ChunkDelay) {
			return
		}
	}
	if !event(chunk(map[string]any{}, "stop")) {
		return
	}
	if !f.NoUsage && body["stream_options"] != nil {
		event(map[string]any{"id": "chatcmpl-fake", "object": "chat.completion.chunk", "model": model, "choices": []any{}, "usage": usage})
	}
	if !f.NoDone {
		out.write("data: [DONE]\n\n")
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// pieces cuts a text into the small parts a model streams.
func pieces(text string) []string {
	var out []string
	words := strings.SplitAfter(text, " ")
	for i := 0; i < len(words); i += 2 {
		out = append(out, strings.Join(words[i:min(i+2, len(words))], ""))
	}
	return out
}

// splitter writes a stream in pieces of n bytes and flushes each one, so that
// a reader sees chunk boundaries anywhere.
type splitter struct {
	w   http.ResponseWriter
	n   int
	ctx context.Context
	buf []byte
}

func (s *splitter) write(text string) bool {
	if s.ctx.Err() != nil {
		return false
	}
	if s.n <= 0 {
		if _, err := io.WriteString(s.w, text); err != nil {
			return false
		}
		s.push()
		return true
	}
	s.buf = append(s.buf, text...)
	for len(s.buf) >= s.n {
		if _, err := s.w.Write(s.buf[:s.n]); err != nil {
			return false
		}
		s.buf = s.buf[s.n:]
		s.push()
		time.Sleep(200 * time.Microsecond)
	}
	return true
}

// finish writes what is left of the last piece.
func (s *splitter) finish() {
	if len(s.buf) > 0 {
		_, _ = s.w.Write(s.buf)
		s.buf = nil
	}
	s.push()
}

func (s *splitter) push() {
	if fl, ok := s.w.(http.Flusher); ok {
		fl.Flush()
	}
}

// Provider is a fake ai.Provider. The zero value answers with CannedAnswer.
type Provider struct {
	ProviderName, ModelName string
	// Deltas, when set, is the answer, piece by piece.
	Deltas []string
	// FinishReason, when set, is the reason the answer reports instead of "stop".
	FinishReason string
	// Err, when set, is returned after ErrAfter deltas were sent.
	Err      error
	ErrAfter int
	// Delay is waited before each delta.
	Delay time.Duration
	// Gate, when set, is waited for before the first delta; closing it lets
	// the answer through. Started is closed when the call is waiting.
	Gate    chan struct{}
	Started chan struct{}

	mu       sync.Mutex
	requests []ai.Request
	once     sync.Once
}

func (p *Provider) Name() string {
	if p.ProviderName != "" {
		return p.ProviderName
	}
	return "fake"
}

func (p *Provider) Model() string {
	if p.ModelName != "" {
		return p.ModelName
	}
	return "fake-model"
}

func (p *Provider) Server() (string, int) { return "fake.invalid", 443 }

// Requests returns the requests the provider got so far.
func (p *Provider) Requests() []ai.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]ai.Request(nil), p.requests...)
}

// Chat implements ai.Provider.
func (p *Provider) Chat(ctx context.Context, req ai.Request, onDelta func(string) error) (ai.Result, error) {
	p.mu.Lock()
	p.requests = append(p.requests, req)
	p.mu.Unlock()
	res := ai.Result{ID: "fake-1", Model: p.Model(), FinishReason: "stop", Usage: ai.Usage{InputTokens: 100, OutputTokens: 20}}
	if p.FinishReason != "" {
		res.FinishReason = p.FinishReason
	}
	if p.Started != nil {
		p.once.Do(func() { close(p.Started) })
	}
	if p.Gate != nil {
		select {
		case <-p.Gate:
		case <-ctx.Done():
			return res, ai.FromTransport(ctx, ctx.Err(), nil)
		}
	}
	deltas := p.Deltas
	if deltas == nil {
		last := ""
		if n := len(req.Messages); n > 0 {
			last = req.Messages[n-1].Content
		}
		deltas = pieces(CannedAnswer(last))
	}
	for i, d := range deltas {
		if p.Err != nil && i == p.ErrAfter {
			return res, p.Err
		}
		if !sleep(ctx, p.Delay) {
			return res, ai.FromTransport(ctx, ctx.Err(), nil)
		}
		if err := onDelta(d); err != nil {
			return res, &ai.Error{Kind: ai.ErrCanceled, Message: "The request was cancelled.", Err: err}
		}
	}
	if p.Err != nil && p.ErrAfter >= len(deltas) {
		return res, p.Err
	}
	return res, nil
}
