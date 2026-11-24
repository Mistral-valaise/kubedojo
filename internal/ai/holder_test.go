package ai_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/Mistral-valaise/kubedojo/internal/ai"
	"github.com/Mistral-valaise/kubedojo/internal/ai/aitest"
	"github.com/Mistral-valaise/kubedojo/internal/ai/netguard"
	"github.com/Mistral-valaise/kubedojo/internal/ai/settings"
)

func effective(model string) settings.Effective {
	e := settings.Effective{}
	e.Enabled.Value = true
	e.Provider.Value = settings.ProviderOpenAI
	e.Model.Value = model
	return e
}

func fakeBuilder(e settings.Effective) (ai.Provider, error) {
	if e.Model.Value == "" {
		return nil, ai.Errorf(ai.ErrNotConfigured, "Choose a model.")
	}
	return &aitest.Provider{ModelName: e.Model.Value}, nil
}

func TestHolderStates(t *testing.T) {
	h := ai.NewHolder(fakeBuilder)
	if p, _ := h.Current(); p != nil || h.Problem() == "" {
		t.Errorf("a new holder has a provider or no problem: %v %q", p, h.Problem())
	}
	if err := h.Apply(effective("a")); err != nil {
		t.Fatal(err)
	}
	p, e := h.Current()
	if p == nil || p.Model() != "a" || e.Model.Value != "a" || h.Problem() != "" {
		t.Errorf("after Apply: %v %+v %q", p, e.Model, h.Problem())
	}
	// Settings no provider can be built from: no provider, the problem in a
	// sentence, and the settings are still reported.
	if err := h.Apply(effective("")); err == nil {
		t.Error("Apply without a model succeeded")
	}
	if p, e := h.Current(); p != nil || !e.Enabled.Value || h.Problem() != "Choose a model." {
		t.Errorf("after a failed Apply: %v %q", p, h.Problem())
	}
	// Switched off.
	off := effective("a")
	off.Enabled.Value = false
	if err := h.Apply(off); err != nil {
		t.Fatal(err)
	}
	if p, _ := h.Current(); p != nil || h.Problem() != "The assistant is switched off." {
		t.Errorf("switched off: %v %q", p, h.Problem())
	}
	// A problem of the configuration itself wins.
	bad := effective("a")
	bad.Problem = "KUBEDOJO_AI=maybe is not a boolean."
	_ = h.Apply(bad)
	if p, _ := h.Current(); p != nil || h.Problem() != bad.Problem {
		t.Errorf("problem: %v %q", p, h.Problem())
	}
}

// Readers always see a provider together with the settings it was built
// from, while the settings change under them. Run with -race.
func TestHolderSwapsUnderConcurrentReaders(t *testing.T) {
	h := ai.NewHolder(fakeBuilder)
	if err := h.Apply(effective("m0")); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				p, e := h.Current()
				if p == nil {
					t.Error("no provider during a swap")
					return
				}
				if p.Model() != e.Model.Value {
					t.Errorf("provider %s with the settings of %s", p.Model(), e.Model.Value)
					return
				}
				_ = h.Problem()
			}
		}()
	}
	for i := range 500 {
		if err := h.Apply(effective(fmt.Sprint("m", i))); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	wg.Wait()
}

// A call that is under way keeps the provider it started with.
func TestHolderInFlightCallKeepsItsProvider(t *testing.T) {
	gate, started := make(chan struct{}), make(chan struct{})
	h := ai.NewHolder(func(e settings.Effective) (ai.Provider, error) {
		p := &aitest.Provider{ModelName: e.Model.Value, Deltas: []string{"from " + e.Model.Value}}
		if e.Model.Value == "old" {
			p.Gate, p.Started = gate, started
		}
		return p, nil
	})
	_ = h.Apply(effective("old"))
	p, _ := h.Current()
	answer := make(chan string, 1)
	go func() {
		var b strings.Builder
		_, _ = p.Chat(context.Background(), ai.Request{}, func(s string) error { b.WriteString(s); return nil })
		answer <- b.String()
	}()
	<-started
	_ = h.Apply(effective("new"))
	close(gate)
	if got := <-answer; got != "from old" {
		t.Errorf("the call under way answered %q", got)
	}
	next, _ := h.Current()
	var b strings.Builder
	_, _ = next.Chat(context.Background(), ai.Request{}, func(s string) error { b.WriteString(s); return nil })
	if b.String() != "from new" {
		t.Errorf("the next call answered %q", b.String())
	}
}

func TestErrorClassification(t *testing.T) {
	ollama, _ := url.Parse("http://127.0.0.1:11434/v1")
	remote, _ := url.Parse("https://api.example.com/v1")
	dial := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for name, tc := range map[string]struct {
		got  *ai.Error
		kind ai.ErrorKind
		msg  string
	}{
		"refused connection":      {ai.FromTransport(context.Background(), dial, remote), ai.ErrUnreachable, "Could not reach api.example.com."},
		"refused, ollama":         {ai.FromTransport(context.Background(), dial, ollama), ai.ErrUnreachable, "Could not reach http://localhost:11434. Is Ollama running?"},
		"no such host":            {ai.FromTransport(context.Background(), &net.DNSError{Err: "no such host", Name: "x", IsNotFound: true}, remote), ai.ErrUnreachable, "Could not reach api.example.com."},
		"deadline":                {ai.FromTransport(context.Background(), context.DeadlineExceeded, remote), ai.ErrTimeout, "The model took too long to answer."},
		"caller gone":             {ai.FromTransport(cancelled, dial, remote), ai.ErrCanceled, "The request was cancelled."},
		"policy":                  {ai.FromTransport(context.Background(), &net.OpError{Op: "dial", Err: &netguard.Error{Reason: "169.254.169.254 is a link-local address"}}, remote), ai.ErrBlockedURL, "The base URL is not allowed: 169.254.169.254 is a link-local address."},
		"broken connection":       {ai.FromTransport(context.Background(), errors.New("unexpected EOF"), remote), ai.ErrUpstream, "The connection to api.example.com was interrupted."},
		"401":                     {ai.FromStatus(401, remote, "m"), ai.ErrAuth, "The provider rejected the API key."},
		"403":                     {ai.FromStatus(403, remote, "m"), ai.ErrAuth, "The provider rejected the API key."},
		"404":                     {ai.FromStatus(404, remote, "m"), ai.ErrModelNotFound, `The model "m" was not found at api.example.com.`},
		"429":                     {ai.FromStatus(429, remote, "m"), ai.ErrRateLimited, "The provider's rate limit was reached. Try again in a moment."},
		"400":                     {ai.FromStatus(400, remote, "m"), ai.ErrBadRequest, "The provider did not accept the request (HTTP 400)."},
		"502":                     {ai.FromStatus(502, remote, "m"), ai.ErrUpstream, "The provider answered with an error (HTTP 502)."},
		"refusal":                 {ai.Refused(), ai.ErrRefused, "The model declined to answer this request."},
		"an error that is not ai": {ai.AsError(errors.New("secret detail")), ai.ErrUpstream, "The assistant could not answer."},
	} {
		if tc.got.Kind != tc.kind || tc.got.Message != tc.msg {
			t.Errorf("%s: %s %q, want %s %q", name, tc.got.Kind, tc.got.Message, tc.kind, tc.msg)
		}
	}
	for kind, want := range map[ai.ErrorKind]bool{
		ai.ErrUnreachable: true, ai.ErrTimeout: true, ai.ErrRateLimited: true, ai.ErrUpstream: true, ai.ErrCanceled: true,
		ai.ErrAuth: false, ai.ErrModelNotFound: false, ai.ErrBadRequest: false, ai.ErrRefused: false, ai.ErrBlockedURL: false, ai.ErrNotConfigured: false,
	} {
		if kind.Retryable() != want {
			t.Errorf("%s retryable = %v", kind, !want)
		}
	}
}
