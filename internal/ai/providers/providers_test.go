package providers

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Mistral-valaise/kubedojo/internal/ai"
	"github.com/Mistral-valaise/kubedojo/internal/ai/netguard"
	"github.com/Mistral-valaise/kubedojo/internal/ai/settings"
)

// noDNS resolves nothing: the tests must not depend on the network.
type noDNS struct{}

func (noDNS) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

func eff(provider, model, baseURL, key string) settings.Effective {
	var e settings.Effective
	e.Enabled.Value = true
	e.Provider.Value, e.Model.Value, e.BaseURL.Value, e.APIKey.Value = provider, model, baseURL, settings.Secret(key)
	return e
}

type wrapped struct{ ai.Provider }

func TestBuild(t *testing.T) {
	f := Factory{Guard: netguard.Policy{SelfAddr: "127.0.0.1:4400", Resolver: noDNS{}}}
	for _, tc := range []struct {
		name     string
		e        settings.Effective
		wantName string
		wantKind ai.ErrorKind
	}{
		{"anthropic", eff("anthropic", "claude-opus-5-5", settings.DefaultAnthropicBaseURL, "key-0000000000000001"), "anthropic", ""},
		{"anthropic without a key", eff("anthropic", "claude-opus-5-5", settings.DefaultAnthropicBaseURL, ""), "", ai.ErrNotConfigured},
		{"ollama needs no key", eff("openai", "llama3.2", "http://localhost:11434/v1", ""), "ollama", ""},
		{"openai", eff("openai", "gpt-x", settings.DefaultOpenAIBaseURL, "key-0000000000000001"), "openai", ""},
		{"openai without a model", eff("openai", "", "http://localhost:11434/v1", ""), "", ai.ErrNotConfigured},
		{"the metadata address", eff("openai", "m", "http://169.254.169.254/v1", ""), "", ai.ErrBlockedURL},
		{"http to a public address", eff("openai", "m", "http://93.184.216.34/v1", ""), "", ai.ErrBlockedURL},
		{"this server", eff("openai", "m", "http://127.0.0.1:4400/v1", ""), "", ai.ErrBlockedURL},
		{"not a URL", eff("openai", "m", "localhost:11434", ""), "", ai.ErrBlockedURL},
		{"unknown provider", eff("gemini", "m", "https://example.com", "k"), "", ai.ErrNotConfigured},
		// The Service of an in-cluster model may not exist yet when the
		// server starts. The client still refuses what is not local.
		{"a name that does not resolve yet", eff("openai", "m", "http://ollama:11434/v1", ""), "ollama", ""},
	} {
		p, err := f.Build(tc.e)
		if tc.wantKind != "" {
			var e *ai.Error
			if !errors.As(err, &e) || e.Kind != tc.wantKind || e.Message == "" {
				t.Errorf("%s: err = %v, want %s", tc.name, err, tc.wantKind)
			}
			continue
		}
		if err != nil || p.Name() != tc.wantName || p.Model() != tc.e.Model.Value {
			t.Errorf("%s: %v, provider %v", tc.name, err, p)
		}
	}

	// A problem of the configuration is the reason, whatever else is set.
	bad := eff("anthropic", "m", settings.DefaultAnthropicBaseURL, "key-0000000000000001")
	bad.Problem = "KUBEDOJO_AI=maybe is not a boolean."
	if _, err := f.Build(bad); ai.AsError(err).Message != bad.Problem {
		t.Errorf("problem: %v", err)
	}

	// Every provider that is built goes through Wrap.
	f.Wrap = func(p ai.Provider) ai.Provider { return wrapped{p} }
	p, err := f.Build(eff("openai", "llama3.2", "http://localhost:11434/v1", ""))
	if _, ok := p.(wrapped); err != nil || !ok {
		t.Errorf("the provider was not wrapped: %T %v", p, err)
	}
}

// rebinding resolves a name to a local address when the URL is checked and
// to the cloud metadata address afterwards.
type rebinding struct {
	mu    sync.Mutex
	asked int
}

func (r *rebinding) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.asked++; r.asked == 1 {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	return []net.IPAddr{{IP: net.ParseIP("169.254.169.254")}}, nil
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// The transport of every provider goes through Transport (telemetry), and
// what is wrapped is the guarded transport: the address a connection is made
// to is still judged, for both providers.
func TestTransportWrapsTheGuardedTransport(t *testing.T) {
	for _, e := range []settings.Effective{
		eff("openai", "m", "http://model.test:8080/v1", ""),
		eff("anthropic", "claude-opus-5-5", "http://model.test:8080", "key-0000000000000001"),
	} {
		t.Run(e.Provider.Value, func(t *testing.T) {
			var seen atomic.Int32
			f := Factory{Guard: netguard.Policy{Resolver: &rebinding{}}}
			f.Transport = func(guarded http.RoundTripper) http.RoundTripper {
				return roundTrip(func(r *http.Request) (*http.Response, error) {
					seen.Add(1)
					return guarded.RoundTrip(r)
				})
			}
			p, err := f.Build(e)
			if err != nil {
				t.Fatal(err)
			}
			_, err = p.Chat(context.Background(), ai.Request{Messages: []ai.Message{{Role: "user", Content: "x"}}}, func(string) error { return nil })
			if ai.AsError(err).Kind != ai.ErrBlockedURL {
				t.Errorf("a name that now resolves to the metadata address: %v, want %s", err, ai.ErrBlockedURL)
			}
			if seen.Load() == 0 {
				t.Error("the request did not go through Transport")
			}
		})
	}
}

// What a user types is judged more strictly than what the deployment sets:
// a name that does not resolve is refused for plain http.
func TestCheckURLIsStrict(t *testing.T) {
	f := Factory{Guard: netguard.Policy{Resolver: noDNS{}}}
	if err := f.CheckURL("http://ollama:11434/v1"); !errors.Is(err, netguard.ErrBlocked) {
		t.Errorf("unresolved http name: %v", err)
	}
	if err := f.CheckURL("http://localhost:11434/v1"); err != nil {
		t.Errorf("localhost: %v", err)
	}
}

func TestList(t *testing.T) {
	l := List()
	if len(l) != 2 || l[0].ID != "anthropic" || !l[0].NeedsKey || l[0].DefaultModel != "claude-opus-5-5" || l[1].ID != "openai" || l[1].NeedsKey || len(l[1].Presets) != 4 {
		t.Errorf("providers = %+v", l)
	}
	for _, p := range l[1].Presets {
		if _, err := (netguard.Policy{Resolver: noDNS{}}).CheckURL(context.Background(), p.BaseURL); err != nil {
			t.Errorf("preset %s is refused by the policy: %v", p.Label, err)
		}
	}
}
