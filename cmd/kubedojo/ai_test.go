package main

import (
	"bytes"
	"context"
	"flag"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mistral-valaise/kubedojo/internal/ai/settings"
	"github.com/Mistral-valaise/kubedojo/internal/store"
)

const cmdTestKey = "sk-cmd-test-key-0123456789abcdWXYZ"

// clearAIEnv makes the test independent of the shell it runs in: a
// developer's own ANTHROPIC_API_KEY must not configure the assistant here.
func clearAIEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"KUBEDOJO_AI", "KUBEDOJO_AI_PROVIDER", "KUBEDOJO_AI_MODEL", "KUBEDOJO_TUTOR_MODEL", "KUBEDOJO_AI_BASE_URL",
		"KUBEDOJO_AI_API_KEY", "KUBEDOJO_AI_TRACE_CONTENT", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL", "OPENAI_API_KEY"} {
		t.Setenv(k, "")
	}
}

func parseAI(t *testing.T, args ...string) (*aiFlags, error) {
	t.Helper()
	fl := flag.NewFlagSet("serve", flag.ContinueOnError)
	fl.SetOutput(io.Discard)
	f := registerAIFlags(fl)
	return f, fl.Parse(args)
}

func TestAIFlags(t *testing.T) {
	f, err := parseAI(t, "--ai", "--ai-provider", "OpenAI", "--ai-model", "llama3.2", "--ai-base-url", "http://localhost:11434/v1", "--ai-trace-content=on")
	if err != nil {
		t.Fatal(err)
	}
	got := f.given()
	want := map[string]string{"ai": "true", "ai-provider": "openai", "ai-model": "llama3.2", "ai-base-url": "http://localhost:11434/v1", "ai-trace-content": "on"}
	if len(got) != len(want) {
		t.Errorf("given = %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	// A flag that was not given is not a source: its default must not lock
	// the setting.
	if f, _ = parseAI(t); len(f.given()) != 0 {
		t.Errorf("without flags: %v", f.given())
	}
	if f, _ = parseAI(t, "--ai=false"); f.given()["ai"] != "false" {
		t.Errorf("--ai=false: %v", f.given())
	}
	// A wrong word fails when the flags are parsed.
	for _, args := range [][]string{{"--ai-provider", "gemini"}, {"--ai-trace-content", "always"}, {"--ai=maybe"}} {
		if _, err := parseAI(t, args...); err == nil {
			t.Errorf("%v was accepted", args)
		}
	}
	// A key on the command line would show in `ps`: there is no flag for it.
	fl := flag.NewFlagSet("serve", flag.ContinueOnError)
	registerAIFlags(fl)
	fl.VisitAll(func(f *flag.Flag) {
		if strings.Contains(f.Name, "key") || strings.Contains(f.Name, "token") || strings.Contains(f.Name, "secret") {
			t.Errorf("flag --%s would take a credential", f.Name)
		}
	})
}

func TestHosted(t *testing.T) {
	for _, tc := range []struct {
		p    serverParts
		want bool
	}{
		{serverParts{sandboxHost: "local"}, false},
		{serverParts{sandboxHost: "local", local: true}, false},
		{serverParts{sandboxHost: "local", userHeader: "X-Forwarded-User"}, true},
		{serverParts{sandboxHost: "kubernetes"}, true},
	} {
		if got := hosted(tc.p); got != tc.want {
			t.Errorf("hosted(%+v) = %v", tc.p, got)
		}
	}
}

func parts(t *testing.T, log io.Writer) serverParts {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "kubedojo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return serverParts{store: st, log: slog.New(slog.NewTextHandler(log, nil)), dataDir: dir, addr: "127.0.0.1:4400", sandboxHost: "local"}
}

// An Anthropic key in the environment and nothing else: the assistant is on,
// as the tutor was.
func TestAssistantFromAnAnthropicKeyAlone(t *testing.T) {
	clearAIEnv(t)
	t.Setenv("ANTHROPIC_API_KEY", cmdTestKey)
	var log bytes.Buffer
	f, _ := parseAI(t)
	svc := f.assistant(parts(t, &log))
	st := svc.Status()
	if !st.Enabled || !st.Configured || st.Provider != "anthropic" || st.Model != "claude-opus-5-5" || !st.Writable {
		t.Errorf("status = %+v", st)
	}
	if !strings.Contains(log.String(), `msg="assistant enabled" provider=anthropic model=claude-opus-5-5`) {
		t.Errorf("start line: %s", log.String())
	}
	if strings.Contains(log.String(), cmdTestKey) {
		t.Error("the key is in the log")
	}
	v := svc.Settings()
	if v.Effective.Enabled.LockedBy != "ANTHROPIC_API_KEY" || v.Effective.APIKey.LockedBy != "ANTHROPIC_API_KEY" || v.Effective.Provider.LockedBy != "ANTHROPIC_API_KEY" {
		t.Errorf("locks = %+v", v.Effective)
	}
}

func TestAssistantWiring(t *testing.T) {
	clearAIEnv(t)

	// Nothing configured: switched off, quietly, and changeable.
	var log bytes.Buffer
	f, _ := parseAI(t)
	p := parts(t, &log)
	svc := f.assistant(p)
	if st := svc.Status(); st.Enabled || st.Configured || !st.Writable {
		t.Errorf("status = %+v", st)
	}
	if log.Len() != 0 {
		t.Errorf("a server without an assistant logs about it: %s", log.String())
	}

	// Flags for a local model: no key needed, and the flags lock.
	log.Reset()
	f, _ = parseAI(t, "--ai", "--ai-provider", "openai", "--ai-base-url", "http://localhost:11434/v1", "--ai-model", "llama3.2")
	svc = f.assistant(p)
	if st := svc.Status(); !st.Configured || st.Provider != "ollama" || st.Model != "llama3.2" {
		t.Errorf("status = %+v", st)
	}
	if e := svc.Settings().Effective; e.Model.LockedBy != "--ai-model" || e.BaseURL.Source != settings.SourceFlag {
		t.Errorf("effective = %+v", e)
	}

	// Switched on but unusable: the reason is logged, once.
	log.Reset()
	f, _ = parseAI(t, "--ai", "--ai-provider", "openai")
	svc = f.assistant(p)
	if st := svc.Status(); st.Configured || !st.Enabled {
		t.Errorf("status = %+v", st)
	}
	if !strings.Contains(log.String(), "assistant not configured") || !strings.Contains(log.String(), "Choose a model.") {
		t.Errorf("log = %s", log.String())
	}

	// The server's own address is refused as a base URL.
	f, _ = parseAI(t, "--ai", "--ai-provider", "openai", "--ai-model", "m", "--ai-base-url", "http://127.0.0.1:4400/v1")
	if svc = f.assistant(p); svc.Status().Configured || !strings.Contains(svc.Settings().Problem, "this kubedojo server itself") {
		t.Errorf("own address: %+v %q", svc.Status(), svc.Settings().Problem)
	}

	// Trace content: recorded for the bundled local stack only, by default.
	f, _ = parseAI(t)
	p.localStack = true
	if v := f.assistant(p).Settings(); !v.TraceContent || v.TraceReason != "local observability stack" {
		t.Errorf("local stack: %v %q", v.TraceContent, v.TraceReason)
	}
	p.userHeader = "X-Forwarded-User"
	if v := f.assistant(p).Settings(); v.TraceContent || v.Writable {
		t.Errorf("hosted: content=%v writable=%v", v.TraceContent, v.Writable)
	}
}

// A hosted server never reads the settings file, and a local one does.
func TestSettingsFileIsForLocalServersOnly(t *testing.T) {
	clearAIEnv(t)
	p := parts(t, io.Discard)
	file := `{"assistant":{"enabled":true,"provider":"openai","model":"from-file","baseURL":"http://localhost:11434/v1"}}`
	if err := os.WriteFile(filepath.Join(p.dataDir, settings.FileName), []byte(file), 0o600); err != nil {
		t.Fatal(err)
	}
	f, _ := parseAI(t)
	if st := f.assistant(p).Status(); !st.Configured || st.Model != "from-file" {
		t.Errorf("local: %+v", st)
	}
	p.sandboxHost = "kubernetes"
	if st := f.assistant(p).Status(); st.Enabled || st.Configured || st.Writable {
		t.Errorf("hosted: %+v", st)
	}
}

// settings.json holds the API key. `kubedojo backup` copies the database and
// nothing else, so a backup that is passed on does not carry the key.
func TestBackupLeavesTheSettingsFileOut(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "kubedojo.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddAssistantMessage(store.AssistantMessage{SessionID: "s", QuestionID: "q", Role: "user", Content: "in the backup"}); err != nil {
		t.Fatal(err)
	}
	st.Close()
	if err := os.WriteFile(filepath.Join(dir, settings.FileName), []byte(`{"assistant":{"apiKey":"`+cmdTestKey+`"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "backup.db")
	if err := cmdBackup(context.Background(), []string{"--data", dir, "--out", out}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Dir(out))
	if len(entries) != 1 {
		t.Errorf("the backup wrote %d files", len(entries))
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(cmdTestKey)) {
		t.Error("the backup contains the API key")
	}
	if !bytes.Contains(raw, []byte("in the backup")) {
		t.Error("the backup lacks the assistant's conversations")
	}
}
