// Package openaicompat talks to any server that implements the OpenAI chat
// completions API: OpenAI itself, Ollama, LM Studio, vLLM, OpenRouter and
// others. It is written against net/http; kubedojo needs one endpoint of that
// API and no SDK for it.
package openaicompat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Mistral-valaise/kubedojo/internal/ai"
)

// Defaults of a provider.
const (
	DefaultMaxTokens      = 2048
	DefaultFirstByte      = 120 * time.Second // a local model may have to be loaded first
	DefaultIdle           = 60 * time.Second
	DefaultTotal          = 180 * time.Second
	DefaultMaxOutputBytes = 32 << 10
)

// Config describes one server and model.
type Config struct {
	// BaseURL is the API root, e.g. http://localhost:11434/v1. The caller
	// has checked it against the base URL policy.
	BaseURL *url.URL
	// APIKey is sent as a bearer token. Empty sends no Authorization header:
	// local servers need none.
	APIKey string
	Model  string
	// HTTPClient makes the requests: the guarded client of the base URL. It
	// must not follow redirects.
	HTTPClient *http.Client
	// MaxTokens is the output limit asked of the server (zero: 2048).
	MaxTokens int
	// FirstByte bounds the wait for the first line of the answer, Idle the
	// wait between two lines, Total the whole call. Zero means the default.
	FirstByte, Idle, Total time.Duration
	// MaxOutputBytes caps the answer text. The call ends with the finish
	// reason "length" when it is reached (zero: 32 KiB).
	MaxOutputBytes int
}

// Provider is an ai.Provider for an OpenAI-compatible server.
type Provider struct {
	cfg      Config
	endpoint string
}

// New returns the provider for cfg.
func New(cfg Config) (*Provider, error) {
	if cfg.BaseURL == nil || cfg.BaseURL.Host == "" {
		return nil, ai.Errorf(ai.ErrNotConfigured, "Set the base URL of the OpenAI-compatible server.")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, ai.Errorf(ai.ErrNotConfigured, "Choose a model.")
	}
	if cfg.HTTPClient == nil {
		return nil, errors.New("openaicompat: no HTTP client")
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = DefaultMaxTokens
	}
	if cfg.FirstByte <= 0 {
		cfg.FirstByte = DefaultFirstByte
	}
	if cfg.Idle <= 0 {
		cfg.Idle = DefaultIdle
	}
	if cfg.Total <= 0 {
		cfg.Total = DefaultTotal
	}
	if cfg.MaxOutputBytes <= 0 {
		cfg.MaxOutputBytes = DefaultMaxOutputBytes
	}
	return &Provider{cfg: cfg, endpoint: strings.TrimRight(cfg.BaseURL.String(), "/") + "/chat/completions"}, nil
}

// NameFor is the provider name of a base URL as OpenTelemetry's
// gen_ai.provider.name knows it, or "openai_compatible".
func NameFor(base *url.URL) string {
	host := strings.ToLower(base.Hostname())
	is := func(domain string) bool { return host == domain || strings.HasSuffix(host, "."+domain) }
	switch {
	case is("api.openai.com"):
		return "openai"
	case is("openrouter.ai"):
		return "openrouter"
	case is("api.groq.com"):
		return "groq"
	case is("api.mistral.ai"):
		return "mistral_ai"
	case is("api.deepseek.com"):
		return "deepseek"
	case is("api.x.ai"):
		return "x_ai"
	case base.Port() == "11434":
		return "ollama"
	}
	return "openai_compatible"
}

func (p *Provider) Name() string  { return NameFor(p.cfg.BaseURL) }
func (p *Provider) Model() string { return p.cfg.Model }

func (p *Provider) Server() (string, int) {
	port, _ := strconv.Atoi(p.cfg.BaseURL.Port())
	if port == 0 {
		port = 443
		if p.cfg.BaseURL.Scheme == "http" {
			port = 80
		}
	}
	return p.cfg.BaseURL.Hostname(), port
}

type wireMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type wireRequest struct {
	Model               string         `json:"model"`
	Messages            []wireMessage  `json:"messages"`
	Stream              bool           `json:"stream"`
	StreamOptions       map[string]any `json:"stream_options,omitempty"`
	MaxTokens           int            `json:"max_tokens,omitempty"`
	MaxCompletionTokens int            `json:"max_completion_tokens,omitempty"`
}

type wireUsage struct {
	PromptTokens        int64 `json:"prompt_tokens"`
	CompletionTokens    int64 `json:"completion_tokens"`
	PromptTokensDetails *struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

// wireChunk is one event of the stream, and also a whole answer of a server
// that does not stream. Only the text is read: reasoning fields are ignored.
type wireChunk struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content json.RawMessage `json:"content"`
		} `json:"delta"`
		Message struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *wireUsage      `json:"usage"`
	Error json.RawMessage `json:"error"`
}

// text returns a content field that is a string; null and other shapes are "".
func text(raw json.RawMessage) string {
	var s string
	if len(raw) == 0 || json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}

// errSlow is the cause when the server kept us waiting too long.
var errSlow = errors.New("the server did not send anything in time")

// errStop ends the reading of a stream that has served its purpose.
var errStop = errors.New("stop")

func unreadable() *ai.Error {
	return &ai.Error{Kind: ai.ErrUpstream, Message: "The provider sent an answer kubedojo could not read."}
}

// Chat sends the conversation and streams the answer text to onDelta.
func (p *Provider) Chat(ctx context.Context, req ai.Request, onDelta func(string) error) (ai.Result, error) {
	caller := ctx
	ctx, cancelTotal := context.WithTimeout(ctx, p.cfg.Total)
	defer cancelTotal()
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	// One watchdog for both waits: for the first line, then between lines.
	watchdog := time.AfterFunc(p.cfg.FirstByte, func() { cancel(errSlow) })
	defer watchdog.Stop()

	// fail turns a transport error into the error of the call.
	fail := func(err error) error {
		if caller.Err() == nil && (errors.Is(context.Cause(ctx), errSlow) || ctx.Err() == context.DeadlineExceeded) {
			return &ai.Error{Kind: ai.ErrTimeout, Message: "The model took too long to answer.", Err: context.Cause(ctx)}
		}
		return ai.FromTransport(caller, err, p.cfg.BaseURL)
	}

	body := wireRequest{Model: p.cfg.Model, Stream: true, StreamOptions: map[string]any{"include_usage": true}, MaxTokens: p.cfg.MaxTokens}
	if req.MaxOutputTokens > 0 {
		body.MaxTokens = req.MaxOutputTokens
	}
	if strings.TrimSpace(req.System) != "" {
		body.Messages = append(body.Messages, wireMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		body.Messages = append(body.Messages, wireMessage{Role: m.Role, Content: m.Content})
	}

	var res *http.Response
	for attempt := 0; ; attempt++ {
		var err error
		if res, err = p.post(ctx, body); err != nil {
			return ai.Result{}, fail(err)
		}
		if res.StatusCode != http.StatusBadRequest || attempt > 0 {
			break
		}
		// Two dialects differ in what they accept. The error text of a 400
		// names the parameter; it is read for that and for nothing else.
		said, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
		res.Body.Close()
		changed := false
		if bytes.Contains(said, []byte("max_completion_tokens")) && body.MaxTokens != 0 {
			body.MaxCompletionTokens, body.MaxTokens = body.MaxTokens, 0
			changed = true
		}
		if bytes.Contains(said, []byte("stream_options")) && body.StreamOptions != nil {
			body.StreamOptions = nil
			changed = true
		}
		if !changed {
			return ai.Result{}, ai.FromStatus(res.StatusCode, p.cfg.BaseURL, p.cfg.Model)
		}
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode >= 300 && res.StatusCode < 400:
		return ai.Result{}, &ai.Error{Kind: ai.ErrUpstream, Status: res.StatusCode,
			Message: "The provider answered with a redirect (HTTP " + strconv.Itoa(res.StatusCode) + "). Redirects are not followed; set the base URL it points to."}
	case res.StatusCode < 200 || res.StatusCode >= 300:
		return ai.Result{}, ai.FromStatus(res.StatusCode, p.cfg.BaseURL, p.cfg.Model)
	}

	out := ai.Result{Model: p.cfg.Model}
	written := 0
	// emit passes text on, up to the output cap.
	emit := func(s string) error {
		if s == "" {
			return nil
		}
		if room := p.cfg.MaxOutputBytes - written; len(s) > room {
			s = s[:max(room, 0)]
			for len(s) > 0 && !utf8.ValidString(s) {
				s = s[:len(s)-1]
			}
			out.FinishReason = "length"
			if s != "" {
				written += len(s)
				if err := onDelta(s); err != nil {
					return err
				}
			}
			return errStop
		}
		written += len(s)
		return onDelta(s)
	}
	// take reads one event or one whole answer.
	take := func(data []byte) error {
		var c wireChunk
		if err := json.Unmarshal(data, &c); err != nil {
			return unreadable()
		}
		if len(c.Error) > 0 && string(c.Error) != "null" {
			return &ai.Error{Kind: ai.ErrUpstream, Message: "The provider reported an error while answering."}
		}
		if c.ID != "" {
			out.ID = c.ID
		}
		if c.Model != "" {
			out.Model = c.Model
		}
		if c.Usage != nil {
			out.Usage.InputTokens, out.Usage.OutputTokens = c.Usage.PromptTokens, c.Usage.CompletionTokens
			if d := c.Usage.PromptTokensDetails; d != nil {
				out.Usage.CacheReadTokens = d.CachedTokens
			}
		}
		if len(c.Choices) == 0 {
			return nil
		}
		ch := c.Choices[0]
		if ch.FinishReason != nil && *ch.FinishReason != "" {
			out.FinishReason = *ch.FinishReason
		}
		return emit(text(ch.Delta.Content) + text(ch.Message.Content))
	}
	finish := func(err error) (ai.Result, error) {
		var aerr *ai.Error
		switch {
		case err == nil || errors.Is(err, errStop):
		case errors.As(err, &aerr):
			return out, aerr
		case caller.Err() != nil || ctx.Err() != nil:
			return out, fail(err)
		default:
			// onDelta failed: whoever was listening is gone.
			return out, &ai.Error{Kind: ai.ErrCanceled, Message: "The request was cancelled.", Err: err}
		}
		if out.FinishReason == "content_filter" {
			return out, ai.Refused()
		}
		if out.FinishReason == "" {
			out.FinishReason = "stop"
		}
		return out, nil
	}

	if ct := res.Header.Get("Content-Type"); strings.HasPrefix(ct, "application/json") {
		// The server ignored "stream": the answer is one JSON document.
		data, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
		if err != nil {
			return out, fail(err)
		}
		return finish(take(data))
	}

	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	var event []byte
	flush := func() error {
		data := bytes.TrimSpace(event)
		event = event[:0]
		if len(data) == 0 {
			return nil
		}
		if string(data) == "[DONE]" {
			return errStop
		}
		return take(data)
	}
	for sc.Scan() {
		watchdog.Reset(p.cfg.Idle)
		line := sc.Bytes()
		switch {
		case len(bytes.TrimSpace(line)) == 0:
			if err := flush(); err != nil {
				return finish(err)
			}
		case bytes.HasPrefix(line, []byte("data:")):
			if len(event) > 0 {
				event = append(event, '\n')
			}
			event = append(event, bytes.TrimPrefix(bytes.TrimPrefix(line, []byte("data:")), []byte(" "))...)
			// Servers send one line per event. A stop marker is acted on at
			// once, also when no blank line follows it.
			if string(bytes.TrimSpace(event)) == "[DONE]" {
				return finish(flush())
			}
		}
		// Comments (": ping") and other fields count as a sign of life only.
	}
	if err := sc.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return finish(unreadable())
		}
		return out, fail(err)
	}
	// The stream ended without a stop marker. What arrived still counts.
	return finish(flush())
}

func (p *Provider) post(ctx context.Context, body wireRequest) (*http.Response, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", "kubedojo")
	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}
	return p.cfg.HTTPClient.Do(req)
}
