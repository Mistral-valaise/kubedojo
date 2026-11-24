package api

import (
	"errors"
	"net/http"

	"github.com/Mistral-valaise/kubedojo/internal/ai/providers"
	"github.com/Mistral-valaise/kubedojo/internal/ai/settings"
	"github.com/Mistral-valaise/kubedojo/internal/assistant"
)

// registerSettings registers the settings endpoints.
func (s *Server) registerSettings(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PUT /api/settings", s.putSettings)
	mux.HandleFunc("POST /api/settings/test", s.testSettings)
}

// fieldView is one setting: its value, where the value comes from, and what
// fixes it if the settings API may not change it.
type fieldView struct {
	Value    any             `json:"value"`
	Source   settings.Source `json:"source"`
	Locked   bool            `json:"locked"`
	LockedBy string          `json:"lockedBy,omitempty"`
}

func field[T any](f settings.Field[T]) fieldView {
	return fieldView{Value: f.Value, Source: f.Source, Locked: f.Locked(), LockedBy: f.LockedBy}
}

// keyView is the API key as the API shows it: that there is one, never what
// it is. Last4 is left out on a hosted server.
type keyView struct {
	Set      bool            `json:"set"`
	Last4    string          `json:"last4,omitempty"`
	Source   settings.Source `json:"source"`
	Locked   bool            `json:"locked"`
	LockedBy string          `json:"lockedBy,omitempty"`
}

type assistantSettingsView struct {
	Enabled    fieldView `json:"enabled"`
	Provider   fieldView `json:"provider"`
	Model      fieldView `json:"model"`
	BaseURL    fieldView `json:"baseURL"`
	APIKey     keyView   `json:"apiKey"`
	Configured bool      `json:"configured"`
	Problem    string    `json:"problem"`
}

type settingsView struct {
	Writable     bool                  `json:"writable"`
	Hosted       bool                  `json:"hosted"`
	Assistant    assistantSettingsView `json:"assistant"`
	Providers    []providers.Info      `json:"providers"`
	TraceContent struct {
		Value  bool   `json:"value"`
		Reason string `json:"reason"`
	} `json:"traceContent"`
}

func viewSettings(v assistant.SettingsView) settingsView {
	e := v.Effective
	out := settingsView{Writable: v.Writable, Hosted: e.Hosted, Providers: v.Providers}
	out.Assistant = assistantSettingsView{Enabled: field(e.Enabled), Provider: field(e.Provider), Model: field(e.Model), BaseURL: field(e.BaseURL),
		APIKey:     keyView{Set: e.APIKey.Value != "", Source: e.APIKey.Source, Locked: e.APIKey.Locked(), LockedBy: e.APIKey.LockedBy},
		Configured: v.Configured}
	if !v.Configured {
		out.Assistant.Problem = v.Problem
	}
	if !e.Hosted {
		out.Assistant.APIKey.Last4 = e.APIKey.Value.Last4()
	}
	out.TraceContent.Value, out.TraceContent.Reason = v.TraceContent, v.TraceReason
	return out
}

// noAssistant answers the settings endpoints of a server that was built
// without the assistant.
func (s *Server) noAssistant(w http.ResponseWriter) bool {
	if s.Assistant != nil {
		return false
	}
	writeErr(w, http.StatusNotFound, "This server has no assistant.")
	return true
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	if s.noAssistant(w) {
		return
	}
	writeJSON(w, 200, viewSettings(s.Assistant.Settings()))
}

// settingsBody is the body of PUT /api/settings and POST /api/settings/test.
// A field that is left out stays as it is; "apiKey": "" removes the key.
type settingsBody struct {
	Assistant struct {
		Enabled  *bool   `json:"enabled"`
		Provider *string `json:"provider"`
		Model    *string `json:"model"`
		BaseURL  *string `json:"baseURL"`
		APIKey   *string `json:"apiKey"`
	} `json:"assistant"`
}

func (b settingsBody) patch() settings.Patch {
	a := b.Assistant
	return settings.Patch{Enabled: a.Enabled, Provider: a.Provider, Model: a.Model, BaseURL: a.BaseURL, APIKey: a.APIKey}
}

// settingsError answers a refused change: 409 for a locked field, 400 for an
// invalid value, both with the name of the field.
func (s *Server) settingsError(w http.ResponseWriter, r *http.Request, err error) {
	var fe *settings.FieldError
	if errors.As(err, &fe) {
		status := http.StatusBadRequest
		if fe.Locked {
			status = http.StatusConflict
		}
		writeCoded(w, status, "", fe.Msg, map[string]any{"field": fe.Field})
		return
	}
	s.refuse(w, r, err)
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	if s.noAssistant(w) || !jsonBody(w, r) {
		return
	}
	var body settingsBody
	if !s.decodeRequired(w, r, &body, 16<<10) {
		return
	}
	view, err := s.Assistant.UpdateSettings(body.patch())
	if err != nil {
		s.settingsError(w, r, err)
		return
	}
	writeJSON(w, 200, viewSettings(view))
}

func (s *Server) testSettings(w http.ResponseWriter, r *http.Request) {
	if s.noAssistant(w) || !jsonBody(w, r) {
		return
	}
	var body settingsBody
	if !s.decode(w, r, &body, 16<<10) {
		return
	}
	res, err := s.Assistant.TestSettings(r.Context(), s.owner(r), body.patch())
	if err != nil {
		s.settingsError(w, r, err)
		return
	}
	writeJSON(w, 200, res)
}
