package ai

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestError(t *testing.T) {
	for _, c := range []struct {
		err  *Error
		want string
	}{
		{&Error{Kind: ErrNotConfigured}, "ai: not_configured"},
		{&Error{Kind: ErrAuth, Status: 401, Message: "The API key was not accepted."}, "ai: auth (HTTP 401): The API key was not accepted."},
		{&Error{Kind: ErrTimeout, Err: context.DeadlineExceeded}, "ai: timeout: context deadline exceeded"},
		{&Error{Kind: ErrUpstream, Status: 529, Message: "The model is overloaded.", Err: errors.New("overloaded_error")},
			"ai: upstream (HTTP 529): The model is overloaded.: overloaded_error"},
	} {
		if got := c.err.Error(); got != c.want {
			t.Errorf("Error() = %q, want %q", got, c.want)
		}
	}

	// The cause stays reachable, and the Error is found behind a wrap.
	err := fmt.Errorf("explain: %w", &Error{Kind: ErrCanceled, Err: context.Canceled})
	if !errors.Is(err, context.Canceled) {
		t.Error("errors.Is does not find the cause")
	}
	var ae *Error
	if !errors.As(err, &ae) || ae.Kind != ErrCanceled {
		t.Errorf("errors.As = %v, want the canceled Error", ae)
	}
	if (&Error{Kind: ErrRefused}).Unwrap() != nil {
		t.Error("Unwrap without a cause is not nil")
	}
}
