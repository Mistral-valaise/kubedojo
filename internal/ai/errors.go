package ai

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
)

// Retryable reports whether sending the same request again can succeed
// without the configuration being changed first.
func (k ErrorKind) Retryable() bool {
	switch k {
	case ErrRateLimited, ErrUnreachable, ErrTimeout, ErrUpstream, ErrCanceled:
		return true
	}
	return false
}

// AsError returns err as an *Error. An error that is none becomes an
// upstream error with a message that says nothing about its cause.
func AsError(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return &Error{Kind: ErrUpstream, Message: "The assistant could not answer.", Err: err}
}

// Errorf returns an *Error of the given kind.
func Errorf(kind ErrorKind, format string, args ...any) *Error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

// hostOf is what a message calls the server: its host name, without a port.
func hostOf(base *url.URL) string {
	if base == nil || base.Hostname() == "" {
		return "the provider"
	}
	// With the port: on one machine it is what tells two servers apart.
	return base.Host
}

func loopbackName(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// FromStatus classifies the HTTP status of an upstream answer that is not a
// success. The body of that answer is never part of the error: it can quote
// the request, and with it the learner's text or a credential.
func FromStatus(status int, base *url.URL, model string) *Error {
	e := &Error{Status: status}
	switch status {
	case 401, 403:
		e.Kind, e.Message = ErrAuth, "The provider rejected the API key."
	case 404:
		e.Kind, e.Message = ErrModelNotFound, fmt.Sprintf("The model %q was not found at %s.", model, hostOf(base))
	case 429:
		e.Kind, e.Message = ErrRateLimited, "The provider's rate limit was reached. Try again in a moment."
	case 400, 422:
		e.Kind, e.Message = ErrBadRequest, fmt.Sprintf("The provider did not accept the request (HTTP %d).", status)
	default:
		e.Kind, e.Message = ErrUpstream, fmt.Sprintf("The provider answered with an error (HTTP %d).", status)
	}
	return e
}

// FromTransport classifies a failure that is not an HTTP status: the caller
// went away, the base URL policy refused, the server could not be reached,
// or it took too long. ctx is the context of the caller of Chat.
func FromTransport(ctx context.Context, err error, base *url.URL) *Error {
	var have *Error
	if errors.As(err, &have) {
		return have
	}
	var guard interface {
		BlockedURL() bool
		Error() string
	}
	var op *net.OpError
	var dns *net.DNSError
	var timeout interface{ Timeout() bool }
	switch {
	case ctx.Err() == context.Canceled:
		return &Error{Kind: ErrCanceled, Message: "The request was cancelled.", Err: context.Canceled}
	case errors.As(err, &guard) && guard.BlockedURL():
		return &Error{Kind: ErrBlockedURL, Message: "The base URL is not allowed: " + guard.Error() + "."}
	case errors.As(err, &dns), errors.As(err, &op) && op.Op == "dial":
		msg := fmt.Sprintf("Could not reach %s.", hostOf(base))
		if base != nil && loopbackName(base.Hostname()) && base.Port() == "11434" {
			msg = "Could not reach http://localhost:11434. Is Ollama running?"
		}
		return &Error{Kind: ErrUnreachable, Message: msg, Err: err}
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &timeout) && timeout.Timeout():
		return &Error{Kind: ErrTimeout, Message: "The model took too long to answer.", Err: err}
	}
	return &Error{Kind: ErrUpstream, Message: fmt.Sprintf("The connection to %s was interrupted.", hostOf(base)), Err: err}
}

// Refused is the error for a model that declined to answer.
func Refused() *Error {
	return &Error{Kind: ErrRefused, Message: "The model declined to answer this request."}
}
