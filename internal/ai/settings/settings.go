// Package settings resolves how the assistant is configured.
//
// Every field has four possible sources, in this order: a command-line flag,
// the environment, the settings file in the data directory, and a default. A
// field whose value comes from a flag or from the environment is locked: the
// settings API reports what locked it and refuses to change it.
//
// The API key is write-only. It is held in a Secret, which prints as
// "[redacted]" wherever it is formatted, logged or serialised.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	"github.com/Mistral-valaise/kubedojo/internal/ai/netguard"
)

// Provider ids.
const (
	ProviderAnthropic = "anthropic"
	ProviderOpenAI    = "openai"
)

// Defaults per provider.
const (
	DefaultAnthropicModel   = "claude-opus-5-5"
	DefaultAnthropicBaseURL = "https://api.anthropic.com"
	DefaultOpenAIBaseURL    = "https://api.openai.com/v1"
)

// Trace content modes (--ai-trace-content).
const (
	TraceAuto = "auto"
	TraceOn   = "on"
	TraceOff  = "off"
)

// FileName is the settings file inside the data directory.
const FileName = "settings.json"

// Source says where the value of a field comes from.
type Source string

const (
	SourceFlag    Source = "flag"
	SourceEnv     Source = "env"
	SourceFile    Source = "file"
	SourceDefault Source = "default"
)

// Secret is a credential. It never prints: formatting, logging and JSON all
// give "[redacted]". Reveal is the one way to its value.
type Secret string

func (s Secret) Reveal() string               { return string(s) }
func (s Secret) String() string               { return "[redacted]" }
func (s Secret) GoString() string             { return "[redacted]" }
func (s Secret) MarshalJSON() ([]byte, error) { return []byte(`"[redacted]"`), nil }
func (s Secret) MarshalText() ([]byte, error) { return []byte("[redacted]"), nil }
func (s Secret) LogValue() slog.Value         { return slog.StringValue("[redacted]") }

// Last4 returns the last four characters of a key that is long enough for
// them not to give it away, and nothing otherwise.
func (s Secret) Last4() string {
	if len(s) < 12 {
		return ""
	}
	return string(s[len(s)-4:])
}

// Field is one resolved setting.
type Field[T any] struct {
	Value  T
	Source Source
	// LockedBy names the flag ("--ai-model") or the environment variable that
	// fixes the value. Empty means the settings API may change it.
	LockedBy string
}

// Locked reports whether the settings API may not change the field.
func (f Field[T]) Locked() bool { return f.LockedBy != "" }

// Effective is the configuration in force.
type Effective struct {
	Enabled  Field[bool]
	Provider Field[string]
	Model    Field[string]
	BaseURL  Field[string]
	APIKey   Field[Secret]
	// AuthToken says the key is a bearer token (ANTHROPIC_AUTH_TOKEN), not
	// an API key.
	AuthToken bool
	// TraceContent is auto, on or off; TraceContentBy names what set it.
	TraceContent   string
	TraceContentBy string
	// Hosted says the server is a shared deployment: the settings file is
	// not used and nothing can be changed at run time.
	Hosted bool
	// Problem is set when the flags or the environment hold a value that
	// cannot be used. The assistant stays off until it is fixed.
	Problem string
}

// Patch is a change to the settings file. A nil field stays as it is. An
// empty APIKey removes the key.
type Patch struct {
	Enabled  *bool
	Provider *string
	Model    *string
	BaseURL  *string
	APIKey   *string
}

// ErrHosted is returned for a change on a hosted server.
var ErrHosted = errors.New("settings are managed by the deployment")

// FieldError says which field of a patch was refused. Locked distinguishes a
// field that may not be changed (409) from a value that is not valid (400).
type FieldError struct {
	Field  string
	Locked bool
	Msg    string
}

func (e *FieldError) Error() string { return e.Msg }

// Inputs is what the resolver reads.
type Inputs struct {
	// Flags holds the flags that were given on the command line, by name
	// without dashes: ai, ai-provider, ai-model, ai-base-url, ai-trace-content.
	Flags map[string]string
	// LookupEnv is os.LookupEnv.
	LookupEnv func(string) (string, bool)
	// Hosted: never read or write the settings file.
	Hosted bool
	// Dir is the data directory.
	Dir string
	// CheckURL validates a base URL that arrives through the settings API.
	// nil accepts every URL.
	CheckURL func(raw string) error
}

// Resolver holds the inputs and the settings file and resolves them.
type Resolver struct {
	in   Inputs
	mu   sync.Mutex
	file fileData
	// loadErr is why the settings file could not be read.
	loadErr error
}

// fileData is the settings file.
type fileData struct {
	Assistant fileAssistant `json:"assistant"`
}

type fileAssistant struct {
	Enabled  *bool  `json:"enabled,omitempty"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	BaseURL  string `json:"baseURL,omitempty"`
	APIKey   string `json:"apiKey,omitempty"`
	// APIKeyFor records what the key was entered for. The key is used only
	// while the provider and the origin of the base URL are still these, so
	// that no other source of configuration can send it elsewhere.
	APIKeyFor *keyBinding `json:"apiKeyFor,omitempty"`
}

type keyBinding struct {
	Provider string `json:"provider"`
	Origin   string `json:"origin"`
}

// New reads the settings file (unless hosted) and returns the resolver. A
// settings file that cannot be read is not fatal: the resolver works without
// it and LoadError says why.
func New(in Inputs) *Resolver {
	if in.LookupEnv == nil {
		in.LookupEnv = func(string) (string, bool) { return "", false }
	}
	r := &Resolver{in: in}
	if !in.Hosted && in.Dir != "" {
		raw, err := os.ReadFile(r.path())
		switch {
		case errors.Is(err, os.ErrNotExist):
		case err != nil:
			r.loadErr = fmt.Errorf("read %s: %w", r.path(), err)
		default:
			if err := json.Unmarshal(raw, &r.file); err != nil {
				// The file holds a key: its content stays out of the error.
				r.file = fileData{}
				r.loadErr = fmt.Errorf("%s is not valid JSON and is ignored", r.path())
			}
		}
	}
	return r
}

func (r *Resolver) path() string { return filepath.Join(r.in.Dir, FileName) }

// LoadError is why the settings file was ignored, or nil.
func (r *Resolver) LoadError() error { return r.loadErr }

// Hosted reports whether the settings can be changed at run time at all.
func (r *Resolver) Hosted() bool { return r.in.Hosted }

// Effective resolves the settings in force.
func (r *Resolver) Effective() Effective {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.resolve(r.file)
}

// env returns the first of the variables that is set to something.
func (r *Resolver) env(names ...string) (value, name string, ok bool) {
	for _, n := range names {
		if v, set := r.in.LookupEnv(n); set && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v), n, true
		}
	}
	return "", "", false
}

func (r *Resolver) flag(name string) (string, bool) {
	v, ok := r.in.Flags[name]
	return v, ok
}

func parseBool(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	}
	return false, false
}

// ValidProvider reports whether id names a provider.
func ValidProvider(id string) bool { return id == ProviderAnthropic || id == ProviderOpenAI }

func defaultBaseURL(provider string) string {
	if provider == ProviderOpenAI {
		return DefaultOpenAIBaseURL
	}
	return DefaultAnthropicBaseURL
}

func defaultModel(provider string) string {
	if provider == ProviderAnthropic {
		return DefaultAnthropicModel
	}
	return ""
}

// keyEnv lists the environment variables that can hold the key of a provider,
// in order.
func keyEnv(provider string) []string {
	if provider == ProviderOpenAI {
		return []string{"KUBEDOJO_AI_API_KEY", "OPENAI_API_KEY"}
	}
	return []string{"KUBEDOJO_AI_API_KEY", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"}
}

// resolve applies the precedence to one state of the settings file.
func (r *Resolver) resolve(file fileData) Effective {
	f := file.Assistant
	if r.in.Hosted {
		f = fileAssistant{}
	}
	e := Effective{Hosted: r.in.Hosted}
	problem := func(format string, args ...any) {
		if e.Problem == "" {
			e.Problem = fmt.Sprintf(format, args...)
		}
	}

	// Provider.
	e.Provider = Field[string]{Value: ProviderAnthropic, Source: SourceDefault}
	if v, ok := r.flag("ai-provider"); ok {
		e.Provider = Field[string]{Value: v, Source: SourceFlag, LockedBy: "--ai-provider"}
	} else if v, name, ok := r.env("KUBEDOJO_AI_PROVIDER"); ok {
		e.Provider = Field[string]{Value: v, Source: SourceEnv, LockedBy: name}
	} else if f.Provider != "" {
		e.Provider = Field[string]{Value: f.Provider, Source: SourceFile}
	}
	if !ValidProvider(e.Provider.Value) {
		by := e.Provider.LockedBy
		if by == "" {
			by = FileName
		}
		problem("%s names the unknown provider %q; use anthropic or openai.", by, e.Provider.Value)
	}
	provider := e.Provider.Value
	// What the file says about the model and the base URL belongs to the
	// provider the file names.
	fileApplies := f.Provider == "" || f.Provider == provider

	// The key of that provider. One from the environment fixes the provider
	// and the base URL as well: the settings API must not be able to send a
	// key of the environment to another host.
	envKey, envKeyName, hasEnvKey := r.env(keyEnv(provider)...)
	if hasEnvKey {
		if e.Provider.LockedBy == "" {
			e.Provider.LockedBy = envKeyName
		}
	}

	// Base URL.
	e.BaseURL = Field[string]{Value: defaultBaseURL(provider), Source: SourceDefault}
	baseEnv := []string{"KUBEDOJO_AI_BASE_URL"}
	if provider == ProviderAnthropic {
		baseEnv = append(baseEnv, "ANTHROPIC_BASE_URL")
	}
	if v, ok := r.flag("ai-base-url"); ok {
		e.BaseURL = Field[string]{Value: v, Source: SourceFlag, LockedBy: "--ai-base-url"}
	} else if v, name, ok := r.env(baseEnv...); ok {
		e.BaseURL = Field[string]{Value: v, Source: SourceEnv, LockedBy: name}
	} else if f.BaseURL != "" && fileApplies && !hasEnvKey {
		e.BaseURL = Field[string]{Value: f.BaseURL, Source: SourceFile}
	}
	if hasEnvKey && e.BaseURL.LockedBy == "" {
		e.BaseURL.LockedBy = envKeyName
	}

	// Key.
	switch {
	case hasEnvKey:
		e.APIKey = Field[Secret]{Value: Secret(envKey), Source: SourceEnv, LockedBy: envKeyName}
		e.AuthToken = envKeyName == "ANTHROPIC_AUTH_TOKEN"
	case f.APIKey != "" && f.APIKeyFor != nil && f.APIKeyFor.Provider == provider && f.APIKeyFor.Origin == originOf(e.BaseURL.Value):
		e.APIKey = Field[Secret]{Value: Secret(f.APIKey), Source: SourceFile}
	default:
		e.APIKey = Field[Secret]{Source: SourceDefault}
	}

	// Model.
	e.Model = Field[string]{Value: defaultModel(provider), Source: SourceDefault}
	if v, ok := r.flag("ai-model"); ok {
		e.Model = Field[string]{Value: v, Source: SourceFlag, LockedBy: "--ai-model"}
	} else if v, name, ok := r.env("KUBEDOJO_AI_MODEL", "KUBEDOJO_TUTOR_MODEL"); ok {
		e.Model = Field[string]{Value: v, Source: SourceEnv, LockedBy: name}
	} else if f.Model != "" && fileApplies {
		e.Model = Field[string]{Value: f.Model, Source: SourceFile}
	}

	// Enabled. An Anthropic key in the environment switches the assistant
	// on, as it always did, when Anthropic is the provider.
	e.Enabled = Field[bool]{Source: SourceDefault}
	if v, ok := r.flag("ai"); ok {
		b, valid := parseBool(v)
		if !valid {
			problem("--ai=%s is not a boolean.", v)
		}
		e.Enabled = Field[bool]{Value: b, Source: SourceFlag, LockedBy: "--ai"}
	} else if v, name, ok := r.env("KUBEDOJO_AI"); ok {
		b, valid := parseBool(v)
		if !valid {
			problem("%s=%s is not a boolean; use true or false.", name, v)
		}
		e.Enabled = Field[bool]{Value: b, Source: SourceEnv, LockedBy: name}
	} else if _, name, ok := r.env("ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"); ok && provider == ProviderAnthropic {
		e.Enabled = Field[bool]{Value: true, Source: SourceEnv, LockedBy: name}
	} else if f.Enabled != nil {
		e.Enabled = Field[bool]{Value: *f.Enabled, Source: SourceFile}
	}

	// Trace content.
	e.TraceContent = TraceAuto
	if v, ok := r.flag("ai-trace-content"); ok {
		e.TraceContent, e.TraceContentBy = strings.ToLower(v), "--ai-trace-content"
	} else if v, name, ok := r.env("KUBEDOJO_AI_TRACE_CONTENT"); ok {
		e.TraceContent, e.TraceContentBy = strings.ToLower(v), name
	}
	switch e.TraceContent {
	case TraceAuto, TraceOn, TraceOff:
	default:
		problem("%s=%s is not valid; use auto, on or off.", e.TraceContentBy, e.TraceContent)
		// Unknown means: do not record.
		e.TraceContent = TraceOff
	}
	return e
}

func originOf(raw string) string { return netguard.OriginOf(raw) }

func bindingOf(e Effective) keyBinding {
	return keyBinding{Provider: e.Provider.Value, Origin: originOf(e.BaseURL.Value)}
}

// Update applies a patch to the settings file and returns the settings that
// are in force afterwards. Nothing is written if the patch is refused.
func (r *Resolver) Update(p Patch) (Effective, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	next, eff, err := r.apply(p)
	if err != nil {
		return Effective{}, err
	}
	if err := r.write(next); err != nil {
		return Effective{}, err
	}
	r.file = next
	return eff, nil
}

// Preview returns the settings a patch would put in force and writes nothing.
// The stored key carries over only if the patch leaves the provider and the
// origin of the base URL alone.
func (r *Resolver) Preview(p Patch) (Effective, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, eff, err := r.apply(p)
	return eff, err
}

func invalid(field, format string, args ...any) *FieldError {
	return &FieldError{Field: field, Msg: fmt.Sprintf(format, args...)}
}

func locked(field, by string) *FieldError {
	return &FieldError{Field: field, Locked: true, Msg: fmt.Sprintf("%s is set by %s and cannot be changed here.", field, by)}
}

func clean(field, v string, max int) (string, *FieldError) {
	v = strings.TrimSpace(v)
	if len(v) > max {
		return "", invalid(field, "%s is too long (at most %d characters).", field, max)
	}
	for _, c := range v {
		if unicode.IsControl(c) {
			return "", invalid(field, "%s contains a control character.", field)
		}
	}
	return v, nil
}

// apply computes the settings file after a patch and what it resolves to.
//
// A patch may repeat the value a locked field already has: a form that sends
// every field is not refused for the ones it did not change.
func (r *Resolver) apply(p Patch) (fileData, Effective, error) {
	if r.in.Hosted {
		return fileData{}, Effective{}, ErrHosted
	}
	cur := r.resolve(r.file)
	next := r.file
	a := &next.Assistant

	// The provider. The file always records which provider its model and
	// base URL are for, also when a flag names the provider.
	target := cur.Provider.Value
	if p.Provider != nil {
		v, ferr := clean("provider", *p.Provider, 32)
		if ferr != nil {
			return next, cur, ferr
		}
		if v != target {
			if cur.Provider.Locked() {
				return next, cur, locked("provider", cur.Provider.LockedBy)
			}
			if !ValidProvider(v) {
				return next, cur, invalid("provider", "Unknown provider %q; use anthropic or openai.", v)
			}
			target = v
		}
	}
	if a.Provider != target {
		if a.Provider != "" || target != cur.Provider.Value {
			// The model and the base URL of another provider mean nothing
			// to this one.
			a.Model, a.BaseURL = "", ""
		}
		a.Provider = target
	}
	// The locks depend on the provider: its key may come from the environment.
	mid := r.resolve(next)

	if p.Model != nil {
		v, ferr := clean("model", *p.Model, 200)
		if ferr != nil {
			return next, cur, ferr
		}
		if v != mid.Model.Value {
			if mid.Model.Locked() {
				return next, cur, locked("model", mid.Model.LockedBy)
			}
			a.Model = v
		}
	}
	if p.BaseURL != nil {
		v, ferr := clean("baseURL", *p.BaseURL, 500)
		if ferr != nil {
			return next, cur, ferr
		}
		if v != mid.BaseURL.Value {
			if mid.BaseURL.Locked() {
				return next, cur, locked("baseURL", mid.BaseURL.LockedBy)
			}
			if v != "" && r.in.CheckURL != nil {
				if err := r.in.CheckURL(v); err != nil {
					return next, cur, invalid("baseURL", "%s", sentence(err.Error()))
				}
			}
			a.BaseURL = v
		}
	}
	if p.Enabled != nil && *p.Enabled != mid.Enabled.Value {
		if mid.Enabled.Locked() {
			return next, cur, locked("enabled", mid.Enabled.LockedBy)
		}
		v := *p.Enabled
		a.Enabled = &v
	}

	// The key belongs to the provider and the origin it was entered for.
	after := r.resolve(next)
	now := bindingOf(after)
	switch {
	case p.APIKey != nil:
		if after.APIKey.Locked() {
			return next, cur, locked("apiKey", after.APIKey.LockedBy)
		}
		v := strings.TrimSpace(*p.APIKey)
		if len(v) > 500 {
			return next, cur, invalid("apiKey", "apiKey is too long (at most 500 characters).")
		}
		for _, c := range v {
			if unicode.IsControl(c) || unicode.IsSpace(c) {
				return next, cur, invalid("apiKey", "apiKey contains white space or a control character.")
			}
		}
		if v == "" {
			a.APIKey, a.APIKeyFor = "", nil
		} else {
			a.APIKey, a.APIKeyFor = v, &now
		}
	case a.APIKeyFor == nil || *a.APIKeyFor != now:
		// Another provider or another server: the key was not entered for it.
		a.APIKey, a.APIKeyFor = "", nil
	}
	return next, r.resolve(next), nil
}

func sentence(s string) string {
	if s == "" {
		return s
	}
	s = strings.ToUpper(s[:1]) + s[1:]
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}

// write stores the settings file: a temporary file in the same directory,
// readable by the owner only, renamed over the old one. A crash leaves either
// the old file or the new one.
func (r *Resolver) write(d fileData) error {
	if r.in.Dir == "" {
		return errors.New("no data directory to store the settings in")
	}
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(r.in.Dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(r.in.Dir, ".settings-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	fail := func(err error) error {
		tmp.Close()
		_ = os.Remove(name)
		return fmt.Errorf("write %s: %w", r.path(), err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		return fail(err)
	}
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("write %s: %w", r.path(), err)
	}
	if err := os.Rename(name, r.path()); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("write %s: %w", r.path(), err)
	}
	return nil
}
