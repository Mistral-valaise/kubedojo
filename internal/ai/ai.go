// Package ai is the provider-neutral contract of the kubedojo assistant.
//
// It has the types only: what kubedojo sends to a model, what it gets back,
// and how a failure is classified. The providers implement Provider; the API
// and telemetry depend on this package and on no provider.
package ai

import (
	"context"
	"strconv"
)

// Message is one turn of a conversation. Role is "user" or "assistant".
type Message struct{ Role, Content string }

// Request is one call to a model.
type Request struct {
	System          string
	Messages        []Message
	MaxOutputTokens int
	ConversationID  string
}

// Usage counts the tokens of one call. The meaning is the same for every
// provider, and it is the one of OpenTelemetry's gen_ai.usage.* attributes:
//
//   - InputTokens is everything the model read: the prompt, including the
//     tokens that came from a prompt cache and those that were written to it.
//   - CacheReadTokens and CacheWriteTokens are parts of InputTokens, not
//     additions to it. They are zero for a provider that does not say.
//   - OutputTokens is everything the model wrote, reasoning included.
//
// A provider whose API counts differently (the Claude API reports cached
// tokens next to its input tokens, not within them) is converted by its
// adapter. All zero means the provider reported no usage.
type Usage struct{ InputTokens, OutputTokens, CacheReadTokens, CacheWriteTokens int64 }

// Result describes a finished call. The text itself went to onDelta.
type Result struct {
	ID, Model, FinishReason string
	Usage                   Usage
}

// Provider is a model behind some API.
type Provider interface {
	// Name is the provider name as used in gen_ai.provider.name, e.g.
	// "anthropic", "openai", "ollama".
	Name() string
	// Model is the model requests are sent to.
	Model() string
	// Server is the host and port of the API the provider talks to.
	Server() (host string, port int)
	// Chat sends the conversation and calls onDelta for each piece of text as it arrives.
	Chat(ctx context.Context, req Request, onDelta func(text string) error) (Result, error)
}

// ErrorKind classifies a failed call independent of the provider.
type ErrorKind string

const (
	ErrNotConfigured ErrorKind = "not_configured"
	ErrAuth          ErrorKind = "auth"
	ErrModelNotFound ErrorKind = "model_not_found"
	ErrRateLimited   ErrorKind = "rate_limited"
	ErrUnreachable   ErrorKind = "unreachable"
	ErrTimeout       ErrorKind = "timeout"
	ErrBadRequest    ErrorKind = "bad_request"
	ErrRefused       ErrorKind = "refused"
	ErrUpstream      ErrorKind = "upstream"
	ErrCanceled      ErrorKind = "canceled"
	ErrBlockedURL    ErrorKind = "blocked_url"
)

// Error is the error of a Provider.
type Error struct {
	Kind    ErrorKind
	Status  int    // upstream HTTP status, 0 if none
	Message string // safe to show to the learner; never an upstream body
	Err     error
}

// Error is the text for logs: the kind, the upstream status, the message and
// the cause. What the learner sees is Message alone.
func (e *Error) Error() string {
	s := "ai: " + string(e.Kind)
	if e.Status != 0 {
		s += " (HTTP " + strconv.Itoa(e.Status) + ")"
	}
	if e.Message != "" {
		s += ": " + e.Message
	}
	if e.Err != nil {
		s += ": " + e.Err.Error()
	}
	return s
}

func (e *Error) Unwrap() error { return e.Err }
