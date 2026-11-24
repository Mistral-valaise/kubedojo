package assistant

import (
	"context"
	"errors"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/ai"
	"github.com/Mistral-valaise/kubedojo/internal/ai/providers"
	"github.com/Mistral-valaise/kubedojo/internal/ai/settings"
)

// SettingsView is the configuration of the assistant as the settings API
// reports it. The key itself is in Effective as a Secret, which does not
// print; the API shows that it is set and, on a local server, its last four
// characters.
type SettingsView struct {
	Effective settings.Effective
	// Configured says a provider could be built; Problem says why not.
	Configured bool
	Problem    string
	// Writable is false on a hosted server.
	Writable  bool
	Providers []providers.Info
	// TraceContent says whether prompts and answers are recorded in traces,
	// and TraceReason why.
	TraceContent bool
	TraceReason  string
}

// TraceContent decides whether prompts and answers may be recorded in
// traces: when it was switched on, or by default exactly when the traces go
// to the bundled local observability stack and the server is not hosted.
func TraceContent(eff settings.Effective, localStack bool) (on bool, reason string) {
	switch eff.TraceContent {
	case settings.TraceOn:
		return true, "switched on with " + eff.TraceContentBy
	case settings.TraceOff:
		if eff.TraceContentBy == "" {
			return false, "switched off"
		}
		return false, "switched off with " + eff.TraceContentBy
	}
	switch {
	case eff.Hosted:
		return false, "hosted server"
	case localStack:
		return true, "local observability stack"
	}
	return false, "no local observability stack"
}

func (s *Service) view() SettingsView {
	p, eff := s.cfg.Holder.Current()
	v := SettingsView{Effective: eff, Configured: p != nil, Problem: s.cfg.Holder.Problem(), Writable: !eff.Hosted, Providers: providers.List()}
	v.TraceContent, v.TraceReason = TraceContent(eff, s.cfg.LocalStack)
	return v
}

// Settings reports the configuration in force.
func (s *Service) Settings() SettingsView {
	s.change.Lock()
	defer s.change.Unlock()
	return s.view()
}

func hosted() *Refusal {
	return &Refusal{Status: 403, Code: CodeHosted, Message: "The assistant of this server is configured by its deployment (flags and environment) and cannot be changed here."}
}

// UpdateSettings changes the settings file and puts the result in force for
// the next message. The error is a *settings.FieldError for a field that was
// refused, or a *Refusal on a hosted server. Settings that are valid but not
// enough for a provider (no key yet, no model yet) are saved: the view then
// says what is missing.
func (s *Service) UpdateSettings(p settings.Patch) (SettingsView, error) {
	s.change.Lock()
	defer s.change.Unlock()
	eff, err := s.cfg.Settings.Update(p)
	if errors.Is(err, settings.ErrHosted) {
		return SettingsView{}, hosted()
	}
	if err != nil {
		return SettingsView{}, err
	}
	if err := s.cfg.Holder.Apply(eff); err != nil {
		s.cfg.Log.Info("assistant: the new settings are not usable yet", "reason", ai.AsError(err).Message)
	} else {
		s.logState()
	}
	return s.view(), nil
}

// logState writes one line about the assistant: what it runs on, or why it
// does not run. Never the key.
func (s *Service) logState() {
	p, eff := s.cfg.Holder.Current()
	switch {
	case p != nil:
		s.cfg.Log.Info("assistant enabled", "provider", p.Name(), "model", p.Model())
	case eff.Enabled.Value || eff.Problem != "":
		s.cfg.Log.Warn("assistant not configured", "reason", s.cfg.Holder.Problem())
	}
}

// LogState is logState for the code that starts the server.
func (s *Service) LogState() { s.logState() }

// TestResult is the outcome of a settings test.
type TestResult struct {
	OK        bool         `json:"ok"`
	Provider  string       `json:"provider,omitempty"`
	Model     string       `json:"model,omitempty"`
	LatencyMs int64        `json:"latencyMs,omitempty"`
	Code      ai.ErrorKind `json:"code,omitempty"`
	Message   string       `json:"message,omitempty"`
}

// testTimeout bounds a settings test.
const testTimeout = 20 * time.Second

// TestSettings sends a tiny request to the provider that the settings in
// force, overlaid with p, describe. It saves nothing. A stored key is used
// only if p leaves the provider and the origin of the base URL alone. A test
// that ran has no error, whatever it found; the error is a
// *settings.FieldError or a *Refusal (hosted, rate limit).
func (s *Service) TestSettings(ctx context.Context, owner string, p settings.Patch) (TestResult, error) {
	eff, err := s.cfg.Settings.Preview(p)
	if errors.Is(err, settings.ErrHosted) {
		return TestResult{}, hosted()
	}
	if err != nil {
		return TestResult{}, err
	}
	if r := s.allow(owner); r != nil {
		return TestResult{}, r
	}
	failed := func(err error) (TestResult, error) {
		e := ai.AsError(err)
		return TestResult{Code: e.Kind, Message: e.Message}, nil
	}
	provider, err := s.cfg.Factory.Build(eff)
	if err != nil {
		return failed(err)
	}
	ctx, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()
	start := time.Now()
	_, err = provider.Chat(ctx, ai.Request{
		System:          "This is a connection test. Reply with the single word: ok",
		Messages:        []ai.Message{{Role: "user", Content: "ping"}},
		MaxOutputTokens: 16,
		ConversationID:  "settings-test",
	}, func(string) error { return nil })
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			err = &ai.Error{Kind: ai.ErrTimeout, Message: "The model did not answer within 20 seconds."}
		}
		s.cfg.Log.InfoContext(ctx, "assistant: settings test failed", "provider", provider.Name(), "model", provider.Model(),
			"kind", string(ai.AsError(err).Kind), "status", ai.AsError(err).Status)
		return failed(err)
	}
	return TestResult{OK: true, Provider: provider.Name(), Model: provider.Model(), LatencyMs: max(time.Since(start).Milliseconds(), 1)}, nil
}
