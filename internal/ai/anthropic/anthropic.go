// Package anthropic talks to the Claude API through Anthropic's Go SDK.
package anthropic

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"github.com/Mistral-valaise/kubedojo/internal/ai"
)

const (
	// DefaultModel is used unless the configuration names another one.
	DefaultModel = "claude-opus-5-5"
	// DefaultBaseURL is the Claude API.
	DefaultBaseURL = "https://api.anthropic.com"
	// DefaultMaxTokens bounds one answer, thinking included.
	DefaultMaxTokens = 16000
	// DefaultTotal bounds one call.
	DefaultTotal = 180 * time.Second
)

// Config describes the API and the model.
type Config struct {
	// APIKey is the credential: an API key or, with AuthToken set, an OAuth
	// token (ANTHROPIC_AUTH_TOKEN). The SDK sends either as a bearer token.
	APIKey    string
	AuthToken bool
	// BaseURL is the API root. nil means the Claude API. The caller has
	// checked it against the base URL policy.
	BaseURL *url.URL
	Model   string
	// HTTPClient makes the requests: the guarded client of the base URL.
	HTTPClient *http.Client
	// Total bounds one call (zero: 180 seconds).
	Total time.Duration
}

// Provider is an ai.Provider for the Claude API.
type Provider struct {
	client sdk.Client
	cfg    Config
}

// New returns the provider for cfg. Nothing is taken from the environment:
// the key, the base URL and the HTTP client are the ones in cfg.
func New(cfg Config) (*Provider, error) {
	if cfg.APIKey == "" {
		return nil, ai.Errorf(ai.ErrNotConfigured, "Set an API key for Anthropic.")
	}
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	if cfg.BaseURL == nil {
		cfg.BaseURL, _ = url.Parse(DefaultBaseURL)
	}
	if cfg.HTTPClient == nil {
		return nil, errors.New("anthropic: no HTTP client")
	}
	if cfg.Total <= 0 {
		cfg.Total = DefaultTotal
	}
	opts := []option.RequestOption{
		// The SDK would otherwise read ANTHROPIC_* itself, and with them a
		// base URL nobody checked.
		option.WithoutEnvironmentDefaults(),
		option.WithBaseURL(cfg.BaseURL.String()),
		option.WithHTTPClient(cfg.HTTPClient),
		option.WithMaxRetries(1),
		// kubedojo emits one provider-neutral span per call itself.
		option.WithoutOpenTelemetry(),
	}
	if cfg.AuthToken {
		opts = append(opts, option.WithAuthToken(cfg.APIKey))
	} else {
		opts = append(opts, option.WithAPIKey(cfg.APIKey))
	}
	return &Provider{client: sdk.NewClient(opts...), cfg: cfg}, nil
}

func (p *Provider) Name() string  { return "anthropic" }
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

// MaxOutputTokens is the output limit a request asks the API for. Telemetry
// records it.
func (p *Provider) MaxOutputTokens(req ai.Request) int {
	if req.MaxOutputTokens > 0 {
		return req.MaxOutputTokens
	}
	return DefaultMaxTokens
}

// supportsFallbacks lists the models whose safety classifiers can decline a
// request and that the API can re-serve on a fallback model.
func supportsFallbacks(model string) bool {
	switch model {
	case "claude-opus-5-5", "claude-opus-5", "claude-fable-5-2", "claude-fable-5-1", "claude-sonnet-5-5":
		return true
	}
	return false
}

// params builds the request. The conversation is replayed as plain text
// turns: kubedojo stores what was said, not the thinking blocks of earlier
// answers, so there is nothing in the history the API would have to verify.
func (p *Provider) params(req ai.Request) sdk.BetaMessageNewParams {
	params := sdk.BetaMessageNewParams{
		Model:        sdk.Model(p.cfg.Model),
		MaxTokens:    int64(p.MaxOutputTokens(req)),
		OutputConfig: sdk.BetaOutputConfigParam{Effort: sdk.BetaOutputConfigEffortMedium},
	}
	if strings.TrimSpace(req.System) != "" {
		// The system prompt never changes, so it is the cached prefix.
		params.System = []sdk.BetaTextBlockParam{{Text: req.System, CacheControl: sdk.NewBetaCacheControlEphemeralParam()}}
	}
	for _, m := range req.Messages {
		block := sdk.NewBetaTextBlock(m.Content)
		if m.Role == "assistant" {
			params.Messages = append(params.Messages, sdk.BetaMessageParam{
				OfAssistant: &sdk.BetaAssistantMessageParam{Content: []sdk.BetaContentBlockParamUnion{block}}})
			continue
		}
		params.Messages = append(params.Messages, sdk.NewBetaUserMessage(block))
	}
	// Models with safety classifiers can decline a request. Let the API
	// re-serve such a request on a fallback model instead of failing.
	if supportsFallbacks(p.cfg.Model) {
		params.Betas = []sdk.AnthropicBeta{sdk.AnthropicBetaServerSideFallback2026_07_01}
		params.Fallbacks = sdk.BetaFallbacksParamOfDefault()
	}
	return params
}

// Chat sends the conversation and streams the answer text to onDelta.
func (p *Provider) Chat(ctx context.Context, req ai.Request, onDelta func(string) error) (ai.Result, error) {
	caller := ctx
	ctx, cancel := context.WithTimeout(ctx, p.cfg.Total)
	defer cancel()

	stream := p.client.Beta.Messages.NewStreaming(ctx, p.params(req))
	defer stream.Close()
	var msg sdk.BetaMessage
	for stream.Next() {
		event := stream.Current()
		if err := msg.Accumulate(event); err != nil {
			return p.result(msg), &ai.Error{Kind: ai.ErrUpstream, Message: "The provider sent an answer kubedojo could not read.", Err: err}
		}
		// Text of every text block, also of the blocks a fallback model
		// continues with: a stream never takes back what it sent.
		if event.Type == "content_block_delta" && event.Delta.Type == "text_delta" && event.Delta.Text != "" {
			if err := onDelta(event.Delta.Text); err != nil {
				return p.result(msg), &ai.Error{Kind: ai.ErrCanceled, Message: "The request was cancelled.", Err: err}
			}
		}
	}
	if err := stream.Err(); err != nil {
		return p.result(msg), p.classify(caller, ctx, err)
	}
	out := p.result(msg)
	// A fallback model can decline as well. Text that was streamed before a
	// refusal is not an answer.
	if msg.StopReason == sdk.BetaStopReasonRefusal {
		return out, ai.Refused()
	}
	return out, nil
}

// result reads what the API said about the call. The Claude API counts the
// tokens it read from and wrote to the prompt cache next to input_tokens;
// ai.Usage counts them within the input tokens, so they are added here.
func (p *Provider) result(msg sdk.BetaMessage) ai.Result {
	out := ai.Result{ID: msg.ID, Model: string(msg.Model), Usage: ai.Usage{
		InputTokens:      msg.Usage.InputTokens + msg.Usage.CacheReadInputTokens + msg.Usage.CacheCreationInputTokens,
		OutputTokens:     msg.Usage.OutputTokens,
		CacheReadTokens:  msg.Usage.CacheReadInputTokens,
		CacheWriteTokens: msg.Usage.CacheCreationInputTokens,
	}}
	if out.Model == "" {
		out.Model = p.cfg.Model
	}
	switch msg.StopReason {
	case sdk.BetaStopReasonEndTurn, sdk.BetaStopReasonStopSequence:
		out.FinishReason = "stop"
	case sdk.BetaStopReasonMaxTokens:
		out.FinishReason = "length"
	default:
		out.FinishReason = string(msg.StopReason)
	}
	return out
}

// classify turns an error of the SDK into the error of the call. An API
// error carries the body of the answer; only its status, its type and the
// request id are kept.
func (p *Provider) classify(caller, ctx context.Context, err error) error {
	var apiErr *sdk.Error
	if errors.As(err, &apiErr) {
		e := ai.FromStatus(apiErr.StatusCode, p.cfg.BaseURL, p.cfg.Model)
		switch apiErr.StatusCode {
		case 404:
			e.Message = fmt.Sprintf("The model %q is not available to this API key.", p.cfg.Model)
		case 529:
			e.Kind, e.Message = ai.ErrRateLimited, "The Claude API is overloaded. Try again in a moment."
		}
		e.Err = fmt.Errorf("anthropic: %s (request %s)", apiErr.Type(), apiErr.RequestID)
		return e
	}
	if caller.Err() == nil && ctx.Err() == context.DeadlineExceeded {
		return &ai.Error{Kind: ai.ErrTimeout, Message: "The model took too long to answer.", Err: context.DeadlineExceeded}
	}
	return ai.FromTransport(caller, err, p.cfg.BaseURL)
}
