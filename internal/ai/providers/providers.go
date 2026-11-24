// Package providers builds the provider a configuration describes and lists
// the providers there are.
package providers

import (
	"context"
	"errors"
	"net/url"

	"github.com/Mistral-valaise/kubedojo/internal/ai"
	"github.com/Mistral-valaise/kubedojo/internal/ai/anthropic"
	"github.com/Mistral-valaise/kubedojo/internal/ai/netguard"
	"github.com/Mistral-valaise/kubedojo/internal/ai/openaicompat"
	"github.com/Mistral-valaise/kubedojo/internal/ai/settings"
)

// Preset is a well-known server of the OpenAI-compatible provider.
type Preset struct {
	Label   string `json:"label"`
	BaseURL string `json:"baseURL"`
}

// Info describes a provider to the settings UI.
type Info struct {
	ID           string   `json:"id"`
	Label        string   `json:"label"`
	NeedsKey     bool     `json:"needsKey"`
	DefaultModel string   `json:"defaultModel,omitempty"`
	Presets      []Preset `json:"presets,omitempty"`
}

// List returns the providers kubedojo has.
func List() []Info {
	return []Info{
		{ID: settings.ProviderAnthropic, Label: "Anthropic", NeedsKey: true, DefaultModel: settings.DefaultAnthropicModel},
		{ID: settings.ProviderOpenAI, Label: "OpenAI-compatible", Presets: []Preset{
			{Label: "Ollama", BaseURL: "http://localhost:11434/v1"},
			{Label: "LM Studio", BaseURL: "http://localhost:1234/v1"},
			{Label: "OpenAI", BaseURL: "https://api.openai.com/v1"},
			{Label: "OpenRouter", BaseURL: "https://openrouter.ai/api/v1"},
		}},
	}
}

// Factory builds providers.
type Factory struct {
	// Guard is the base URL policy of this server.
	Guard netguard.Policy
	// Wrap is put around every provider that is built (telemetry). nil
	// leaves it as it is.
	Wrap func(ai.Provider) ai.Provider
}

// CheckURL judges a base URL that somebody entered: strictly, a name that
// does not resolve is refused for plain http.
func (f Factory) CheckURL(raw string) error {
	_, err := f.Guard.CheckURL(context.Background(), raw)
	return err
}

// baseURL judges the base URL of a configuration. A host name of an http URL
// that does not resolve at the moment is let through: the server may simply
// not be up yet, and the client refuses every address that is not local when
// it connects.
func (f Factory) baseURL(raw string) (*url.URL, error) {
	u, err := f.Guard.CheckURL(context.Background(), raw)
	if err == nil {
		return u, nil
	}
	var ge *netguard.Error
	if errors.As(err, &ge) && ge.Unresolved {
		if u, perr := url.Parse(raw); perr == nil {
			return u, nil
		}
	}
	return nil, &ai.Error{Kind: ai.ErrBlockedURL, Message: "The base URL is not allowed: " + err.Error() + "."}
}

// Build returns the provider for eff. The error is an *ai.Error whose message
// says in one sentence what is missing.
func (f Factory) Build(eff settings.Effective) (ai.Provider, error) {
	if eff.Problem != "" {
		return nil, ai.Errorf(ai.ErrNotConfigured, "%s", eff.Problem)
	}
	var (
		p   ai.Provider
		err error
	)
	switch eff.Provider.Value {
	case settings.ProviderAnthropic:
		if eff.APIKey.Value == "" {
			return nil, ai.Errorf(ai.ErrNotConfigured, "Set an API key for Anthropic.")
		}
		base, berr := f.baseURL(eff.BaseURL.Value)
		if berr != nil {
			return nil, berr
		}
		p, err = anthropic.New(anthropic.Config{APIKey: eff.APIKey.Value.Reveal(), AuthToken: eff.AuthToken, BaseURL: base,
			Model: eff.Model.Value, HTTPClient: f.Guard.Client(base, netguard.ClientOptions{})})
	case settings.ProviderOpenAI:
		if eff.Model.Value == "" {
			return nil, ai.Errorf(ai.ErrNotConfigured, "Choose a model.")
		}
		base, berr := f.baseURL(eff.BaseURL.Value)
		if berr != nil {
			return nil, berr
		}
		p, err = openaicompat.New(openaicompat.Config{APIKey: eff.APIKey.Value.Reveal(), BaseURL: base,
			Model: eff.Model.Value, HTTPClient: f.Guard.Client(base, netguard.ClientOptions{})})
	default:
		return nil, ai.Errorf(ai.ErrNotConfigured, "Unknown provider %q; use anthropic or openai.", eff.Provider.Value)
	}
	if err != nil {
		return nil, err
	}
	if f.Wrap != nil {
		p = f.Wrap(p)
	}
	return p, nil
}
