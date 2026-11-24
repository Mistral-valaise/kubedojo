package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const testKey = "sk-test-0123456789abcdefWXYZ"

func envOf(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func ptr[T any](v T) *T { return &v }

func writeFile(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, dir string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestDefaults(t *testing.T) {
	e := New(Inputs{Dir: t.TempDir()}).Effective()
	if e.Enabled.Value || e.Enabled.Source != SourceDefault || e.Enabled.Locked() {
		t.Errorf("enabled = %+v", e.Enabled)
	}
	if e.Provider.Value != ProviderAnthropic || e.Model.Value != DefaultAnthropicModel || e.BaseURL.Value != DefaultAnthropicBaseURL {
		t.Errorf("defaults = %s %s %s", e.Provider.Value, e.Model.Value, e.BaseURL.Value)
	}
	if e.APIKey.Value != "" || e.TraceContent != TraceAuto || e.Problem != "" || e.Hosted {
		t.Errorf("unexpected: %+v", e)
	}
}

// Each field takes the flag over the environment over the file over the
// default, and the first two lock it.
func TestPrecedence(t *testing.T) {
	file := `{"assistant":{"enabled":true,"provider":"openai","model":"file-model","baseURL":"http://localhost:1234/v1"}}`
	type want struct {
		enabled            bool
		provider, model    string
		baseURL            string
		eSrc, pSrc, mSrc   Source
		bSrc               Source
		eBy, pBy, mBy, bBy string
	}
	for _, tc := range []struct {
		name  string
		flags map[string]string
		env   map[string]string
		file  string
		want  want
	}{
		{"file only", nil, nil, file, want{true, "openai", "file-model", "http://localhost:1234/v1",
			SourceFile, SourceFile, SourceFile, SourceFile, "", "", "", ""}},
		{"env over file", nil, map[string]string{"KUBEDOJO_AI": "false", "KUBEDOJO_AI_PROVIDER": "openai", "KUBEDOJO_AI_MODEL": "env-model", "KUBEDOJO_AI_BASE_URL": "http://localhost:11434/v1"}, file,
			want{false, "openai", "env-model", "http://localhost:11434/v1", SourceEnv, SourceEnv, SourceEnv, SourceEnv,
				"KUBEDOJO_AI", "KUBEDOJO_AI_PROVIDER", "KUBEDOJO_AI_MODEL", "KUBEDOJO_AI_BASE_URL"}},
		{"flag over env", map[string]string{"ai": "true", "ai-provider": "openai", "ai-model": "flag-model", "ai-base-url": "http://127.0.0.1:9/v1"},
			map[string]string{"KUBEDOJO_AI": "false", "KUBEDOJO_AI_PROVIDER": "anthropic", "KUBEDOJO_AI_MODEL": "env-model", "KUBEDOJO_AI_BASE_URL": "http://localhost:11434/v1"}, file,
			want{true, "openai", "flag-model", "http://127.0.0.1:9/v1", SourceFlag, SourceFlag, SourceFlag, SourceFlag,
				"--ai", "--ai-provider", "--ai-model", "--ai-base-url"}},
		{"mixed", map[string]string{"ai-model": "flag-model"}, map[string]string{"KUBEDOJO_AI_PROVIDER": "openai"}, file,
			want{true, "openai", "flag-model", "http://localhost:1234/v1", SourceFile, SourceEnv, SourceFlag, SourceFile,
				"", "KUBEDOJO_AI_PROVIDER", "--ai-model", ""}},
		{"the legacy model variable", nil, map[string]string{"KUBEDOJO_TUTOR_MODEL": "claude-sonnet-5-5"}, "",
			want{false, "anthropic", "claude-sonnet-5-5", DefaultAnthropicBaseURL, SourceDefault, SourceDefault, SourceEnv, SourceDefault,
				"", "", "KUBEDOJO_TUTOR_MODEL", ""}},
		{"the new model variable wins over the legacy one", nil, map[string]string{"KUBEDOJO_TUTOR_MODEL": "old", "KUBEDOJO_AI_MODEL": "new"}, "",
			want{false, "anthropic", "new", DefaultAnthropicBaseURL, SourceDefault, SourceDefault, SourceEnv, SourceDefault,
				"", "", "KUBEDOJO_AI_MODEL", ""}},
		{"an empty variable is not set", nil, map[string]string{"KUBEDOJO_AI_MODEL": "", "KUBEDOJO_AI": " "}, file,
			want{true, "openai", "file-model", "http://localhost:1234/v1", SourceFile, SourceFile, SourceFile, SourceFile, "", "", "", ""}},
		{"the file's model belongs to the file's provider", map[string]string{"ai-provider": "anthropic"}, nil, file,
			want{true, "anthropic", DefaultAnthropicModel, DefaultAnthropicBaseURL, SourceFile, SourceFlag, SourceDefault, SourceDefault,
				"", "--ai-provider", "", ""}},
		{"ANTHROPIC_BASE_URL is for anthropic", nil, map[string]string{"ANTHROPIC_BASE_URL": "https://proxy.example.com"}, "",
			want{false, "anthropic", DefaultAnthropicModel, "https://proxy.example.com", SourceDefault, SourceDefault, SourceDefault, SourceEnv,
				"", "", "", "ANTHROPIC_BASE_URL"}},
		{"ANTHROPIC_BASE_URL is not for openai", map[string]string{"ai-provider": "openai"}, map[string]string{"ANTHROPIC_BASE_URL": "https://proxy.example.com"}, "",
			want{false, "openai", "", DefaultOpenAIBaseURL, SourceDefault, SourceFlag, SourceDefault, SourceDefault,
				"", "--ai-provider", "", ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.file != "" {
				writeFile(t, dir, tc.file)
			}
			e := New(Inputs{Flags: tc.flags, LookupEnv: envOf(tc.env), Dir: dir}).Effective()
			got := want{e.Enabled.Value, e.Provider.Value, e.Model.Value, e.BaseURL.Value,
				e.Enabled.Source, e.Provider.Source, e.Model.Source, e.BaseURL.Source,
				e.Enabled.LockedBy, e.Provider.LockedBy, e.Model.LockedBy, e.BaseURL.LockedBy}
			if got != tc.want {
				t.Errorf("\n got %+v\nwant %+v", got, tc.want)
			}
			if e.Problem != "" {
				t.Errorf("problem: %s", e.Problem)
			}
		})
	}
}

// An Anthropic key in the environment is all the old tutor needed. That must
// keep working with nothing else configured.
func TestAnthropicKeyAloneEnablesTheAssistant(t *testing.T) {
	for _, name := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"} {
		e := New(Inputs{LookupEnv: envOf(map[string]string{name: testKey}), Dir: t.TempDir()}).Effective()
		if !e.Enabled.Value || e.Enabled.Source != SourceEnv || e.Enabled.LockedBy != name {
			t.Errorf("%s: enabled = %+v", name, e.Enabled)
		}
		if e.Provider.Value != ProviderAnthropic || e.Model.Value != DefaultAnthropicModel {
			t.Errorf("%s: %s %s", name, e.Provider.Value, e.Model.Value)
		}
		if e.APIKey.Value.Reveal() != testKey || e.APIKey.Source != SourceEnv || e.APIKey.LockedBy != name {
			t.Errorf("%s: key field = %+v", name, e.APIKey)
		}
		if e.AuthToken != (name == "ANTHROPIC_AUTH_TOKEN") {
			t.Errorf("%s: AuthToken = %v", name, e.AuthToken)
		}
	}
	// KUBEDOJO_AI=false still switches it off.
	e := New(Inputs{LookupEnv: envOf(map[string]string{"ANTHROPIC_API_KEY": testKey, "KUBEDOJO_AI": "false"})}).Effective()
	if e.Enabled.Value {
		t.Error("KUBEDOJO_AI=false did not win over the implied default")
	}
	// With another provider chosen the Anthropic key means nothing.
	e = New(Inputs{Flags: map[string]string{"ai-provider": "openai"}, LookupEnv: envOf(map[string]string{"ANTHROPIC_API_KEY": testKey})}).Effective()
	if e.Enabled.Value || e.APIKey.Value != "" {
		t.Errorf("an Anthropic key configured the openai provider: %+v", e)
	}
}

func TestKeySources(t *testing.T) {
	for _, tc := range []struct {
		provider string
		env      map[string]string
		wantBy   string
		wantKey  string
	}{
		{"anthropic", map[string]string{"KUBEDOJO_AI_API_KEY": "generic-key-00000001", "ANTHROPIC_API_KEY": "anthropic-key-0000002"}, "KUBEDOJO_AI_API_KEY", "generic-key-00000001"},
		{"anthropic", map[string]string{"ANTHROPIC_API_KEY": "anthropic-key-0000002", "ANTHROPIC_AUTH_TOKEN": "token-000000000003"}, "ANTHROPIC_API_KEY", "anthropic-key-0000002"},
		{"anthropic", map[string]string{"OPENAI_API_KEY": "openai-key-000000004"}, "", ""},
		{"openai", map[string]string{"OPENAI_API_KEY": "openai-key-000000004", "ANTHROPIC_API_KEY": "anthropic-key-0000002"}, "OPENAI_API_KEY", "openai-key-000000004"},
		{"openai", map[string]string{"KUBEDOJO_AI_API_KEY": "generic-key-00000001", "OPENAI_API_KEY": "openai-key-000000004"}, "KUBEDOJO_AI_API_KEY", "generic-key-00000001"},
		{"openai", nil, "", ""},
	} {
		e := New(Inputs{Flags: map[string]string{"ai-provider": tc.provider}, LookupEnv: envOf(tc.env)}).Effective()
		if e.APIKey.Value.Reveal() != tc.wantKey || e.APIKey.LockedBy != tc.wantBy {
			t.Errorf("%s %v: key from %q, want %q", tc.provider, tc.env, e.APIKey.LockedBy, tc.wantBy)
		}
	}
}

// A key of the environment must not be sendable to a host the settings API
// chose: it locks the provider and the base URL, and the base URL of the
// settings file is not used with it.
func TestEnvironmentKeyLocksProviderAndBaseURL(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, `{"assistant":{"provider":"openai","baseURL":"https://collector.example.com/v1","model":"m"}}`)
	r := New(Inputs{LookupEnv: envOf(map[string]string{"OPENAI_API_KEY": testKey}), Dir: dir})
	e := r.Effective()
	if e.BaseURL.Value != DefaultOpenAIBaseURL {
		t.Fatalf("the key of the environment would go to %s", e.BaseURL.Value)
	}
	if e.Provider.LockedBy != "OPENAI_API_KEY" || e.BaseURL.LockedBy != "OPENAI_API_KEY" || e.APIKey.LockedBy != "OPENAI_API_KEY" {
		t.Errorf("locks: provider=%q baseURL=%q key=%q", e.Provider.LockedBy, e.BaseURL.LockedBy, e.APIKey.LockedBy)
	}
	for field, p := range map[string]Patch{
		"provider": {Provider: ptr("anthropic")},
		"baseURL":  {BaseURL: ptr("https://collector.example.com/v1")},
		"apiKey":   {APIKey: ptr("another-key-000000000")},
	} {
		var fe *FieldError
		if _, err := r.Update(p); !errors.As(err, &fe) || !fe.Locked || fe.Field != field {
			t.Errorf("%s: err = %v", field, err)
		}
		if _, err := r.Preview(p); !errors.As(err, &fe) || !fe.Locked {
			t.Errorf("%s: a preview got around the lock: %v", field, err)
		}
	}
	// The model is not locked by the key.
	if e, err := r.Update(Patch{Model: ptr("gpt-x")}); err != nil || e.Model.Value != "gpt-x" {
		t.Errorf("model: %v %+v", err, e.Model)
	}
	// An explicit base URL of the environment stays in force.
	e = New(Inputs{LookupEnv: envOf(map[string]string{"OPENAI_API_KEY": testKey, "KUBEDOJO_AI_PROVIDER": "openai", "KUBEDOJO_AI_BASE_URL": "http://ollama:11434/v1"})}).Effective()
	if e.BaseURL.Value != "http://ollama:11434/v1" || e.BaseURL.LockedBy != "KUBEDOJO_AI_BASE_URL" {
		t.Errorf("base URL = %+v", e.BaseURL)
	}
}

// A key stored for one provider is never used for another one that a flag
// selects later.
func TestStoredKeyIsBoundToProviderAndOrigin(t *testing.T) {
	dir := t.TempDir()
	r := New(Inputs{Dir: dir})
	if _, err := r.Update(Patch{Enabled: ptr(true), Provider: ptr("openai"), BaseURL: ptr("http://localhost:11434/v1"), Model: ptr("llama3.2"), APIKey: ptr(testKey)}); err != nil {
		t.Fatal(err)
	}
	for name, in := range map[string]Inputs{
		"another provider by flag": {Flags: map[string]string{"ai-provider": "anthropic"}, Dir: dir},
		"another host by flag":     {Flags: map[string]string{"ai-base-url": "https://collector.example.com/v1"}, Dir: dir},
		"another host by env":      {LookupEnv: envOf(map[string]string{"KUBEDOJO_AI_BASE_URL": "http://localhost:9999/v1"}), Dir: dir},
	} {
		if e := New(in).Effective(); e.APIKey.Value != "" {
			t.Errorf("%s: the stored key is used for %s %s", name, e.Provider.Value, e.BaseURL.Value)
		}
	}
	// Same origin, other path: still the same server.
	e := New(Inputs{Flags: map[string]string{"ai-base-url": "http://localhost:11434/api/v1"}, Dir: dir}).Effective()
	if e.APIKey.Value.Reveal() != testKey {
		t.Error("the key was dropped although the origin is the same")
	}
}

func TestUpdate(t *testing.T) {
	dir := t.TempDir()
	checked := []string{}
	r := New(Inputs{Dir: dir, CheckURL: func(raw string) error {
		checked = append(checked, raw)
		if strings.Contains(raw, "metadata") {
			return errors.New("that address is not allowed")
		}
		return nil
	}})

	e, err := r.Update(Patch{Enabled: ptr(true), Provider: ptr("openai"), Model: ptr(" llama3.2 "), BaseURL: ptr("http://localhost:11434/v1"), APIKey: ptr(testKey)})
	if err != nil {
		t.Fatal(err)
	}
	if !e.Enabled.Value || e.Provider.Value != "openai" || e.Model.Value != "llama3.2" || e.BaseURL.Value != "http://localhost:11434/v1" || e.APIKey.Value.Reveal() != testKey {
		t.Fatalf("after the update: %+v", e)
	}
	for _, f := range []Source{e.Enabled.Source, e.Provider.Source, e.Model.Source, e.BaseURL.Source, e.APIKey.Source} {
		if f != SourceFile {
			t.Errorf("source = %s, want file", f)
		}
	}
	// A new resolver reads the same from disk.
	if e2 := New(Inputs{Dir: dir}).Effective(); e2 != e {
		t.Errorf("reloaded:\n got %+v\nwant %+v", e2, e)
	}

	// An omitted field stays.
	if e, err = r.Update(Patch{Model: ptr("qwen3")}); err != nil || e.Model.Value != "qwen3" || e.APIKey.Value.Reveal() != testKey || !e.Enabled.Value {
		t.Fatalf("partial update: %v %+v", err, e)
	}
	// Another path on the same server keeps the key.
	if e, err = r.Update(Patch{BaseURL: ptr("http://localhost:11434/api/v1")}); err != nil || e.APIKey.Value.Reveal() != testKey {
		t.Fatalf("same origin: %v, key kept = %v", err, e.APIKey.Value != "")
	}
	// Another server without a new key drops it.
	if e, err = r.Update(Patch{BaseURL: ptr("http://localhost:1234/v1")}); err != nil || e.APIKey.Value != "" {
		t.Fatalf("other origin: %v, key kept = %v", err, e.APIKey.Value != "")
	}
	if strings.Contains(readFile(t, dir), testKey) {
		t.Error("the dropped key is still in the settings file")
	}
	// Another server with a new key takes the new key.
	if e, err = r.Update(Patch{BaseURL: ptr("http://localhost:11434/v1"), APIKey: ptr(testKey)}); err != nil || e.APIKey.Value.Reveal() != testKey {
		t.Fatalf("other origin with a key: %v", err)
	}
	// Another provider without a new key drops it, and the old provider's
	// model and base URL with it.
	if e, err = r.Update(Patch{Provider: ptr("anthropic")}); err != nil {
		t.Fatal(err)
	}
	if e.APIKey.Value != "" || e.Model.Value != DefaultAnthropicModel || e.BaseURL.Value != DefaultAnthropicBaseURL {
		t.Fatalf("after the provider change: %+v", e)
	}
	// "" removes the key.
	if _, err = r.Update(Patch{APIKey: ptr(testKey)}); err != nil {
		t.Fatal(err)
	}
	if e, err = r.Update(Patch{APIKey: ptr("")}); err != nil || e.APIKey.Value != "" || strings.Contains(readFile(t, dir), testKey) {
		t.Fatalf("removing the key: %v", err)
	}

	// Refusals name the field and write nothing.
	before := readFile(t, dir)
	for field, p := range map[string]Patch{
		"provider": {Provider: ptr("gemini")},
		"baseURL":  {BaseURL: ptr("http://metadata.internal/")},
		"model":    {Model: ptr(strings.Repeat("m", 201))},
		"apiKey":   {APIKey: ptr("has a space")},
	} {
		var fe *FieldError
		if _, err := r.Update(p); !errors.As(err, &fe) || fe.Locked || fe.Field != field {
			t.Errorf("%s: err = %#v", field, err)
		}
	}
	if readFile(t, dir) != before {
		t.Error("a refused update changed the settings file")
	}
	if len(checked) == 0 {
		t.Error("the base URL was never checked")
	}
}

func TestLockedFields(t *testing.T) {
	dir := t.TempDir()
	r := New(Inputs{Dir: dir,
		Flags:     map[string]string{"ai": "true", "ai-provider": "openai"},
		LookupEnv: envOf(map[string]string{"KUBEDOJO_AI_MODEL": "env-model", "KUBEDOJO_AI_BASE_URL": "http://localhost:11434/v1"})})
	for field, tc := range map[string]struct {
		p  Patch
		by string
	}{
		"enabled":  {Patch{Enabled: ptr(false)}, "--ai"},
		"provider": {Patch{Provider: ptr("anthropic")}, "--ai-provider"},
		"model":    {Patch{Model: ptr("other")}, "KUBEDOJO_AI_MODEL"},
		"baseURL":  {Patch{BaseURL: ptr("http://localhost:1/v1")}, "KUBEDOJO_AI_BASE_URL"},
	} {
		var fe *FieldError
		_, err := r.Update(tc.p)
		if !errors.As(err, &fe) || !fe.Locked || fe.Field != field || !strings.Contains(fe.Msg, tc.by) {
			t.Errorf("%s: err = %#v", field, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); !errors.Is(err, os.ErrNotExist) {
		t.Error("a refused update created the settings file")
	}
	// A form that repeats the locked values and changes an open field is fine.
	e, err := r.Update(Patch{Enabled: ptr(true), Provider: ptr("openai"), Model: ptr("env-model"), BaseURL: ptr("http://localhost:11434/v1"), APIKey: ptr(testKey)})
	if err != nil || e.APIKey.Value.Reveal() != testKey {
		t.Fatalf("repeating locked values: %v", err)
	}
}

func TestPreviewWritesNothing(t *testing.T) {
	dir := t.TempDir()
	r := New(Inputs{Dir: dir})
	if _, err := r.Update(Patch{Provider: ptr("openai"), BaseURL: ptr("http://localhost:11434/v1"), Model: ptr("a"), APIKey: ptr(testKey)}); err != nil {
		t.Fatal(err)
	}
	before := readFile(t, dir)
	e, err := r.Preview(Patch{Model: ptr("b")})
	if err != nil || e.Model.Value != "b" || e.APIKey.Value.Reveal() != testKey {
		t.Fatalf("preview with the stored key: %v %+v", err, e.Model)
	}
	// The stored key does not follow to another server or provider.
	if e, _ = r.Preview(Patch{BaseURL: ptr("http://localhost:9/v1")}); e.APIKey.Value != "" {
		t.Error("the stored key would be sent to another server")
	}
	if e, _ = r.Preview(Patch{Provider: ptr("anthropic")}); e.APIKey.Value != "" {
		t.Error("the stored key would be sent to another provider")
	}
	if e, _ = r.Preview(Patch{BaseURL: ptr("http://localhost:9/v1"), APIKey: ptr("fresh-key-0000000001")}); e.APIKey.Value.Reveal() != "fresh-key-0000000001" {
		t.Error("the key of the request was not used")
	}
	if readFile(t, dir) != before || r.Effective().Model.Value != "a" {
		t.Error("a preview changed the settings")
	}
}

// Hosted: the file is neither read nor written, whatever it holds.
func TestHostedNeverTouchesTheFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, `{"assistant":{"enabled":true,"provider":"openai","model":"from-file","baseURL":"http://localhost:1/v1"}}`)
	before := readFile(t, dir)
	r := New(Inputs{Hosted: true, Dir: dir})
	e := r.Effective()
	if e.Enabled.Value || e.Provider.Value != ProviderAnthropic || e.Model.Value == "from-file" || !e.Hosted {
		t.Errorf("the settings file was read in hosted mode: %+v", e)
	}
	if _, err := r.Update(Patch{Enabled: ptr(true)}); !errors.Is(err, ErrHosted) {
		t.Errorf("update: %v", err)
	}
	if _, err := r.Preview(Patch{Enabled: ptr(true)}); !errors.Is(err, ErrHosted) {
		t.Errorf("preview: %v", err)
	}
	if readFile(t, dir) != before {
		t.Error("the settings file changed in hosted mode")
	}
	// A hosted server without any file writes none either.
	empty := t.TempDir()
	_, _ = New(Inputs{Hosted: true, Dir: empty}).Update(Patch{Enabled: ptr(true)})
	if entries, _ := os.ReadDir(empty); len(entries) != 0 {
		t.Errorf("hosted mode wrote %v", entries)
	}
}

func TestFileIsPrivateAndWrittenAtomically(t *testing.T) {
	dir := t.TempDir()
	r := New(Inputs{Dir: dir})
	for i := range 3 {
		if _, err := r.Update(Patch{Model: ptr(fmt.Sprintf("m%d", i)), APIKey: ptr(testKey)}); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != FileName {
		t.Errorf("temporary files were left behind: %v", entries)
	}
	if runtime.GOOS != "windows" {
		info, _ := entries[0].Info()
		if info.Mode().Perm() != 0o600 {
			t.Errorf("mode = %o, want 600", info.Mode().Perm())
		}
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(readFile(t, dir)), &parsed); err != nil {
		t.Errorf("the settings file is not JSON: %v", err)
	}

	// A write that fails leaves the old file as it was.
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		before := readFile(t, dir)
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
		if _, err := r.Update(Patch{Model: ptr("lost")}); err == nil {
			t.Error("a write into a read-only directory succeeded")
		}
		if readFile(t, dir) != before || r.Effective().Model.Value == "lost" {
			t.Error("a failed write changed the settings")
		}
	}
}

func TestBadValuesAreProblemsNotCrashes(t *testing.T) {
	for name, in := range map[string]Inputs{
		"provider by env":   {LookupEnv: envOf(map[string]string{"KUBEDOJO_AI_PROVIDER": "gemini"})},
		"enabled by env":    {LookupEnv: envOf(map[string]string{"KUBEDOJO_AI": "maybe"})},
		"trace content env": {LookupEnv: envOf(map[string]string{"KUBEDOJO_AI_TRACE_CONTENT": "always"})},
	} {
		e := New(in).Effective()
		if e.Problem == "" {
			t.Errorf("%s: no problem reported", name)
		}
		if name == "trace content env" && e.TraceContent != TraceOff {
			t.Errorf("an unknown trace mode records content: %s", e.TraceContent)
		}
	}
	// A broken settings file is ignored, and its content is not quoted.
	dir := t.TempDir()
	writeFile(t, dir, `{"assistant":{"apiKey":"`+testKey+`",`)
	r := New(Inputs{Dir: dir})
	if r.LoadError() == nil || strings.Contains(r.LoadError().Error(), testKey) {
		t.Errorf("load error = %v", r.LoadError())
	}
	if e := r.Effective(); e.Enabled.Value || e.APIKey.Value != "" {
		t.Errorf("a broken file was used: %+v", e)
	}
}

func TestTraceContent(t *testing.T) {
	e := New(Inputs{Flags: map[string]string{"ai-trace-content": "on"}, LookupEnv: envOf(map[string]string{"KUBEDOJO_AI_TRACE_CONTENT": "off"})}).Effective()
	if e.TraceContent != TraceOn || e.TraceContentBy != "--ai-trace-content" {
		t.Errorf("flag: %s by %s", e.TraceContent, e.TraceContentBy)
	}
	e = New(Inputs{LookupEnv: envOf(map[string]string{"KUBEDOJO_AI_TRACE_CONTENT": "OFF"})}).Effective()
	if e.TraceContent != TraceOff || e.TraceContentBy != "KUBEDOJO_AI_TRACE_CONTENT" {
		t.Errorf("env: %s by %s", e.TraceContent, e.TraceContentBy)
	}
}

// However a Secret is printed, logged or serialised, the key is not in it.
func TestSecretNeverPrints(t *testing.T) {
	e := New(Inputs{LookupEnv: envOf(map[string]string{"ANTHROPIC_API_KEY": testKey})}).Effective()
	var log bytes.Buffer
	slog.New(slog.NewJSONHandler(&log, nil)).Info("settings", "effective", e, "key", e.APIKey.Value, "field", e.APIKey)
	slog.New(slog.NewTextHandler(&log, nil)).Info("settings", "effective", e, "key", e.APIKey.Value, "field", e.APIKey)
	js, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	for name, out := range map[string]string{
		"%v":   fmt.Sprintf("%v", e),
		"%+v":  fmt.Sprintf("%+v", e),
		"%#v":  fmt.Sprintf("%#v", e),
		"%s":   fmt.Sprintf("%s", e.APIKey.Value),
		"%q":   fmt.Sprintf("%q", e.APIKey.Value),
		"%x":   fmt.Sprintf("%x", e.APIKey.Value),
		"json": string(js),
		"slog": log.String(),
	} {
		if strings.Contains(out, testKey) || strings.Contains(out, fmt.Sprintf("%x", testKey)) {
			t.Errorf("%s prints the key: %s", name, out)
		}
	}
	if got := e.APIKey.Value.Last4(); got != "WXYZ" {
		t.Errorf("Last4 = %q", got)
	}
	if got := Secret("short").Last4(); got != "" {
		t.Errorf("Last4 of a short key = %q", got)
	}
}
