package api_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/ai"
	"github.com/Mistral-valaise/kubedojo/internal/ai/aitest"
	"github.com/Mistral-valaise/kubedojo/internal/ai/netguard"
	"github.com/Mistral-valaise/kubedojo/internal/ai/providers"
	"github.com/Mistral-valaise/kubedojo/internal/ai/settings"
	"github.com/Mistral-valaise/kubedojo/internal/api"
	"github.com/Mistral-valaise/kubedojo/internal/assistant"
	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox/sandboxtest"
	"github.com/Mistral-valaise/kubedojo/internal/session"
	"github.com/Mistral-valaise/kubedojo/internal/store"
)

// secretKey is a key that must never show up in a response or in the log.
const secretKey = "sk-leak-canary-7f3a9c2e5d1b8a64WXYZ"

type logBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *logBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// asst is a kubedojo API server with an assistant that talks to a fake
// OpenAI-compatible server, through the real provider and the real policy.
type asst struct {
	t      *testing.T
	url    string
	fake   *aitest.Fake
	fakeAt string // base URL of the fake server
	dir    string
	logs   *logBuffer
	// bodies collects every response body the helpers read.
	mu     sync.Mutex
	bodies []string
}

type asstOptions struct {
	// flags of the assistant; nil configures the fake server.
	flags map[string]string
	env   map[string]string
	// hosted puts the server behind a proxy that names the user.
	hosted bool
	fake   *aitest.Fake
	ping   time.Duration
}

func newAsst(t *testing.T, o asstOptions) *asst {
	t.Helper()
	a := &asst{t: t, fake: o.fake, dir: t.TempDir(), logs: &logBuffer{}}
	if a.fake == nil {
		a.fake = &aitest.Fake{}
	}
	upstream := httptest.NewServer(a.fake)
	t.Cleanup(upstream.Close)
	a.fakeAt = upstream.URL + "/v1"
	if o.flags == nil && o.env == nil {
		o.flags = map[string]string{"ai": "true", "ai-provider": "openai", "ai-model": "fake", "ai-base-url": a.fakeAt}
	}

	f := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }
	head := "exam: {durationMinutes: 10, passPercent: 50, questions: 1}\ndomains: [{id: a, title: A, weight: 100}]\n"
	cat, err := catalog.Load(fstest.MapFS{
		"quiz/track.yaml":        f("title: Quiz\nshort: QZ\nkind: theory\ndocs: [{title: C, url: \"https://example.org/c\"}]\n" + head),
		"quiz/lessons/a.md":      f("# A"),
		"quiz/questions/a1.yaml": f(quizQuestion("a")),
		"k/track.yaml":           f("title: K\nkubernetesVersion: v1.37\n" + head + "environments: {base: {workers: 1}}\n"),
		"k/lessons/a.md":         f("# A"),
		"k/questions/q/question.yaml": f("title: Scale it\ndomain: a\ntask: Scale web on {{host}}.\nhints: [look]\nchecks:\n" +
			"  - {id: c, title: T, type: resource, resource: deployment/web, namespace: shop, explain: e}\n"),
		"k/questions/q/setup.sh":    f("true"),
		"k/questions/q/solution.sh": f("true"),
		"k/questions/q/solution.md": f("scale it"),
	})
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(a.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mgr, err := session.NewManager(session.Config{Catalog: cat, Store: st, Driver: &sandboxtest.Driver{}, Logger: log})
	if err != nil {
		t.Fatal(err)
	}

	// The wiring of cmd/kubedojo, with the test's flags and environment.
	factory := providers.Factory{Guard: netguard.Policy{}}
	resolver := settings.New(settings.Inputs{Flags: o.flags, LookupEnv: func(k string) (string, bool) { v, ok := o.env[k]; return v, ok },
		Hosted: o.hosted, Dir: a.dir, CheckURL: factory.CheckURL})
	holder := ai.NewHolder(factory.Build)
	_ = holder.Apply(resolver.Effective())
	svc := assistant.New(assistant.Config{Holder: holder, Settings: resolver, Factory: factory, Store: st, Sessions: mgr, Catalog: cat,
		Log: log, Render: api.Render, PingInterval: o.ping})
	svc.LogState()

	s := &api.Server{Catalog: cat, Sessions: mgr, Store: st, Log: log, Version: "test", Assistant: svc}
	if o.hosted {
		s.UserHeader = "X-User"
	}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(func() {
		srv.Close()
		mgr.Close()
		st.Close()
	})
	a.url = srv.URL
	return a
}

func (a *asst) record(body string) {
	a.mu.Lock()
	a.bodies = append(a.bodies, body)
	a.mu.Unlock()
}

// do sends a JSON request as the user "alice" and returns the response with
// its body read.
func (a *asst) do(method, path, body string) (*http.Response, string) {
	a.t.Helper()
	req, err := http.NewRequest(method, a.url+path, strings.NewReader(body))
	if err != nil {
		a.t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-User", "alice")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	a.record(string(raw))
	return res, string(raw)
}

// json is do for an answer that is a JSON object.
func (a *asst) json(method, path, body string, want int) map[string]any {
	a.t.Helper()
	res, raw := a.do(method, path, body)
	if res.StatusCode != want {
		a.t.Fatalf("%s %s = %d %s, want %d", method, path, res.StatusCode, raw, want)
	}
	var v map[string]any
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			a.t.Fatalf("%s %s: not JSON: %s", method, path, raw)
		}
	}
	return v
}

func (a *asst) session(mode, track string) string {
	a.t.Helper()
	v := a.json("POST", "/api/sessions", `{"mode":"`+mode+`","track":"`+track+`"}`, 201)
	id, _ := v["id"].(string)
	if id == "" {
		a.t.Fatal("no session id")
	}
	return id
}

type sseEvent struct {
	Name string
	Data map[string]any
}

// readSSE parses a stream into its events and also returns the raw text.
func readSSE(r io.Reader, each func(sseEvent)) ([]sseEvent, string) {
	var events []sseEvent
	var raw strings.Builder
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 1<<20)
	var cur sseEvent
	for sc.Scan() {
		line := sc.Text()
		raw.WriteString(line + "\n")
		switch {
		case strings.HasPrefix(line, "event: "):
			cur.Name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			_ = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &cur.Data)
		case line == "" && cur.Name != "":
			events = append(events, cur)
			if each != nil {
				each(cur)
			}
			cur = sseEvent{}
		}
	}
	return events, raw.String()
}

// stream posts a message and reads the whole event stream.
func (a *asst) stream(sid, qid, body string) (*http.Response, []sseEvent, string) {
	a.t.Helper()
	req, _ := http.NewRequest("POST", a.url+"/api/sessions/"+sid+"/questions/"+qid+"/assistant", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User", "alice")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		raw, _ := io.ReadAll(res.Body)
		a.t.Fatalf("POST assistant = %d %s", res.StatusCode, raw)
	}
	events, raw := readSSE(res.Body, nil)
	a.record(raw)
	return res, events, raw
}

func names(events []sseEvent) string {
	var out []string
	for _, e := range events {
		n := e.Name
		if n == "context" {
			n += ":" + e.Data["stage"].(string)
		}
		if n == "delta" && len(out) > 0 && out[len(out)-1] == "delta" {
			continue
		}
		out = append(out, n)
	}
	return strings.Join(out, " ")
}

func deltas(events []sseEvent) string {
	var b strings.Builder
	for _, e := range events {
		if e.Name == "delta" {
			b.WriteString(e.Data["text"].(string))
		}
	}
	return b.String()
}

func TestAssistantStream(t *testing.T) {
	a := newAsst(t, asstOptions{})
	sid := a.session("practice", "quiz")
	base := "/api/sessions/" + sid + "/questions/a1/assistant"

	// Before anything was said.
	h := a.json("GET", base, "", 200)
	if len(h["messages"].([]any)) != 0 {
		t.Errorf("history of a new conversation: %v", h)
	}
	if l := h["limits"].(map[string]any); l["turns"] != float64(20) || l["messageChars"] != float64(4000) {
		t.Errorf("limits = %v", l)
	}

	res, events, _ := a.stream(sid, "a1", `{"message":"Which one is it?"}`)
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q", ct)
	}
	if res.Header.Get("Cache-Control") != "no-store" || res.Header.Get("X-Accel-Buffering") != "no" {
		t.Errorf("headers = %v", res.Header)
	}
	if got := names(events); got != "meta context:checks delta done" {
		t.Fatalf("events = %s", got)
	}
	meta := events[0].Data
	if meta["conversation"] != sid+"/a1" || meta["provider"] != "openai_compatible" || meta["model"] != "fake" || meta["turn"] != float64(1) || meta["userMessageId"] == nil {
		t.Errorf("meta = %v", meta)
	}
	answer := deltas(events)
	if answer != aitest.CannedAnswer("Which one is it?") {
		t.Errorf("answer = %q", answer)
	}
	done := events[len(events)-1].Data
	usage, _ := done["usage"].(map[string]any)
	if done["messageId"] == nil || done["finishReason"] != "stop" || usage["inputTokens"] == float64(0) || usage["outputTokens"] == float64(0) || done["durationMs"] == nil {
		t.Errorf("done = %v", done)
	}

	// The model got the system prompt and the context in front of the message.
	sent := a.fake.Requests()[0]
	msgs := sent.Body["messages"].([]any)
	if len(msgs) != 2 || msgs[0].(map[string]any)["content"] != ai.System {
		t.Fatalf("sent = %v", msgs)
	}
	if last := sent.LastUser(); !strings.HasPrefix(last, "<context>") || !strings.Contains(last, "Pick the right one on") || !strings.HasSuffix(last, "\n\nWhich one is it?") {
		t.Errorf("the newest message = %q", last)
	}
	if strings.Contains(sent.LastUser(), keyExplanation) {
		t.Error("the answer key was sent to the model before the question was revealed")
	}

	// The conversation is stored.
	h = a.json("GET", base, "", 200)
	stored := h["messages"].([]any)
	if len(stored) != 2 {
		t.Fatalf("history = %v", stored)
	}
	q, r := stored[0].(map[string]any), stored[1].(map[string]any)
	if q["role"] != "user" || q["content"] != "Which one is it?" || q["id"] != meta["userMessageId"] || q["partial"] != false {
		t.Errorf("stored question = %v", q)
	}
	if r["role"] != "assistant" || r["content"] != answer || r["id"] != done["messageId"] || r["partial"] != false {
		t.Errorf("stored answer = %v", r)
	}
	if _, err := time.Parse(time.RFC3339, r["at"].(string)); err != nil {
		t.Errorf("at = %v: %v", r["at"], err)
	}

	// The second turn carries the first as plain text, and can be retried.
	_, events, _ = a.stream(sid, "a1", `{"message":"And why?"}`)
	if events[0].Data["turn"] != float64(2) {
		t.Errorf("turn = %v", events[0].Data["turn"])
	}
	msgs = a.fake.Requests()[1].Body["messages"].([]any)
	if len(msgs) != 4 || msgs[1].(map[string]any)["content"] != "Which one is it?" || msgs[2].(map[string]any)["content"] != answer {
		t.Errorf("history sent = %v", msgs)
	}
	retry := events[len(events)-1].Data["messageId"].(float64)
	_, events, _ = a.stream(sid, "a1", fmt.Sprintf(`{"retryOf":%d}`, int64(retry)))
	if events[0].Data["turn"] != float64(2) || events[len(events)-1].Data["messageId"] == retry {
		t.Errorf("retry: %v", events)
	}
	if n := len(a.json("GET", base, "", 200)["messages"].([]any)); n != 4 {
		t.Errorf("%d messages after a retry, want 4", n)
	}

	// Clearing.
	if res, _ := a.do("DELETE", base, ""); res.StatusCode != 204 {
		t.Errorf("DELETE = %d", res.StatusCode)
	}
	if n := len(a.json("GET", base, "", 200)["messages"].([]any)); n != 0 {
		t.Errorf("%d messages after DELETE", n)
	}
	// Another user's session does not exist for this one.
	if res, _ := a.do("GET", "/api/sessions/nope/questions/a1/assistant", ""); res.StatusCode != 404 {
		t.Errorf("unknown session = %d", res.StatusCode)
	}
}

// While the model takes its time, the stream sends comment lines.
func TestAssistantStreamPings(t *testing.T) {
	a := newAsst(t, asstOptions{fake: &aitest.Fake{FirstByteDelay: 400 * time.Millisecond}, ping: 40 * time.Millisecond})
	sid := a.session("practice", "quiz")
	_, events, raw := a.stream(sid, "a1", `{"message":"x"}`)
	if n := strings.Count(raw, ": ping\n"); n < 3 {
		t.Errorf("%d pings in a stream that waited 400ms with a ping every 40ms", n)
	}
	if events[len(events)-1].Name != "done" {
		t.Errorf("last event = %s", events[len(events)-1].Name)
	}
}

// The learner closes the page: the call to the model is cancelled, what
// arrived is kept as a partial answer, and the question is free again.
func TestAssistantClientDisconnectCancelsTheModelCall(t *testing.T) {
	a := newAsst(t, asstOptions{fake: &aitest.Fake{StallAfter: 3}})
	sid := a.session("practice", "quiz")
	path := a.url + "/api/sessions/" + sid + "/questions/a1/assistant"

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "POST", path, strings.NewReader(`{"message":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	got := 0
	readSSE(res.Body, func(e sseEvent) {
		// While the answer is running, a second message for the same
		// question is refused.
		if e.Name == "delta" {
			if got++; got == 2 {
				busy := a.json("POST", "/api/sessions/"+sid+"/questions/a1/assistant", `{"message":"again"}`, 409)
				if busy["code"] != "busy" {
					t.Errorf("second stream: %v", busy)
				}
				cancel()
			}
		}
	})
	res.Body.Close()

	deadline := time.Now().Add(3 * time.Second)
	for a.fake.ClientsGone() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if a.fake.ClientsGone() != 1 {
		t.Fatal("the request to the model was not cancelled")
	}
	// The partial answer is stored and the question is free again.
	var stored []any
	for time.Now().Before(deadline) {
		stored = a.json("GET", "/api/sessions/"+sid+"/questions/a1/assistant", "", 200)["messages"].([]any)
		if len(stored) == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(stored) != 2 || stored[1].(map[string]any)["partial"] != true || stored[1].(map[string]any)["content"] == "" {
		t.Fatalf("stored = %v", stored)
	}
	// The question is free again: the next message is admitted.
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	req, _ = http.NewRequestWithContext(ctx2, "POST", path, strings.NewReader(`{"message":"once more"}`))
	req.Header.Set("Content-Type", "application/json")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Errorf("the next message = %d, want it admitted", res.StatusCode)
	}
}

func TestAssistantErrorsBeforeTheStream(t *testing.T) {
	a := newAsst(t, asstOptions{})
	sid := a.session("practice", "quiz")
	base := "/api/sessions/" + sid + "/questions/a1/assistant"
	for name, tc := range map[string]struct {
		path, body string
		status     int
		code       string
	}{
		"not JSON":          {base, `{"message":`, 400, ""},
		"too long":          {base, `{"message":"` + strings.Repeat("a", 4001) + `"}`, 400, "too_long"},
		"body over 8 KiB":   {base, `{"message":"` + strings.Repeat("a", 9000) + `"}`, 400, ""},
		"unknown session":   {"/api/sessions/nope/questions/a1/assistant", `{"message":"x"}`, 404, ""},
		"unknown question":  {"/api/sessions/" + sid + "/questions/nope/assistant", `{"message":"x"}`, 404, ""},
		"unknown retry":     {base, `{"retryOf":424242}`, 404, "no_such_message"},
		"retry of nothing":  {base, `{"retryOf":-1}`, 404, "no_such_message"},
		"a field of a type": {base, `{"message":42}`, 400, ""},
	} {
		v := a.json("POST", tc.path, tc.body, tc.status)
		if v["error"] == nil || v["error"] == "" || tc.code != "" && v["code"] != tc.code {
			t.Errorf("%s: %v", name, v)
		}
	}
	if n := len(a.fake.Requests()); n != 0 {
		t.Errorf("%d refused requests reached the model", n)
	}
	if n := len(a.json("GET", base, "", 200)["messages"].([]any)); n != 0 {
		t.Errorf("%d messages of refused requests were stored", n)
	}

	// A form of another origin cannot send JSON without asking first.
	req, _ := http.NewRequest("POST", a.url+base, strings.NewReader(`{"message":"x"}`))
	req.Header.Set("Content-Type", "text/plain")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("text/plain = %d, want 415", res.StatusCode)
	}

	// The eleventh message within a minute.
	for i := range 10 {
		if res, raw := a.do("POST", base, `{"message":"x"}`); res.StatusCode != 200 {
			t.Fatalf("message %d = %d %s", i+1, res.StatusCode, raw)
		}
	}
	res, raw := a.do("POST", base, `{"message":"x"}`)
	if res.StatusCode != 429 || res.Header.Get("Retry-After") == "" || !strings.Contains(raw, `"code":"rate_limited"`) {
		t.Errorf("11th message = %d %s (Retry-After %q)", res.StatusCode, raw, res.Header.Get("Retry-After"))
	}
}

// The assistant stays out of a running exam.
func TestAssistantIsBlockedDuringAnExam(t *testing.T) {
	a := newAsst(t, asstOptions{})
	sid := a.session("exam", "quiz")
	base := "/api/sessions/" + sid
	a.json("POST", base+"/start", "", 200)
	v := a.json("POST", base+"/questions/a1/assistant", `{"message":"help"}`, 409)
	if v["code"] != "exam_running" {
		t.Errorf("during the exam: %v", v)
	}
	if n := len(a.json("GET", base+"/questions/a1/assistant", "", 200)["messages"].([]any)); n != 0 {
		t.Errorf("history during an exam has %d messages", n)
	}
	if len(a.fake.Requests()) != 0 {
		t.Error("the model was asked during an exam")
	}
	a.json("POST", base+"/finish", "", 200)
	if _, events, _ := a.stream(sid, "a1", `{"message":"explain"}`); events[len(events)-1].Name != "done" {
		t.Errorf("after the exam: %s", names(events))
	}
	// After the exam the answer key is part of the context.
	if !strings.Contains(a.fake.Requests()[0].LastUser(), keyExplanation) {
		t.Error("after the exam the model does not get the answer key")
	}
}

func TestAssistantNotConfigured(t *testing.T) {
	a := newAsst(t, asstOptions{flags: map[string]string{}})
	sid := a.session("practice", "quiz")
	v := a.json("POST", "/api/sessions/"+sid+"/questions/a1/assistant", `{"message":"x"}`, 409)
	if v["code"] != "not_configured" || !strings.Contains(v["error"].(string), "switched off") {
		t.Errorf("not configured: %v", v)
	}
	cfg := a.json("GET", "/api/config", "", 200)
	as := cfg["assistant"].(map[string]any)
	if cfg["tutor"] != false || as["enabled"] != false || as["configured"] != false || as["writable"] != true {
		t.Errorf("config = %v", cfg)
	}
	// A server built without the assistant answers the same way.
	bare := newQuizAPI(t, nil)
	sid2 := bare.session("practice")
	if code, raw := bare.call("POST", "/api/sessions/"+sid2+"/questions/a1/assistant", `{"message":"x"}`); code != 409 || !strings.Contains(raw, `"not_configured"`) {
		t.Errorf("a message to a bare server = %d %s", code, raw)
	}
	if code, raw := bare.call("GET", "/api/sessions/"+sid2+"/questions/a1/assistant", ""); code != 200 || !strings.Contains(raw, `"messages":[]`) {
		t.Errorf("history on a bare server = %d %s", code, raw)
	}
	if code, raw := bare.call("GET", "/api/config", ""); code != 200 || !strings.Contains(raw, `"tutor":false`) || !strings.Contains(raw, `"configured":false`) {
		t.Errorf("config of a bare server = %d %s", code, raw)
	}
	if code, _ := bare.call("GET", "/api/settings", ""); code != 404 {
		t.Errorf("settings of a bare server = %d", code)
	}
}

// A failure after the stream began is an event, with our words for it.
func TestAssistantErrorEvent(t *testing.T) {
	a := newAsst(t, asstOptions{fake: &aitest.Fake{Status: 401}})
	sid := a.session("practice", "quiz")
	_, events, raw := a.stream(sid, "a1", `{"message":"x"}`)
	if got := names(events); got != "meta context:checks error" {
		t.Fatalf("events = %s", got)
	}
	e := events[len(events)-1].Data
	if e["code"] != "auth" || e["message"] != "The provider rejected the API key." || e["retryable"] != false || e["messageId"] != nil {
		t.Errorf("error event = %v", e)
	}
	if strings.Contains(raw, aitest.BodyMarker) || strings.Contains(a.logs.String(), aitest.BodyMarker) {
		t.Error("the upstream body reached the client or the log")
	}
	if !strings.Contains(a.logs.String(), "kind=auth") || !strings.Contains(a.logs.String(), "status=401") {
		t.Errorf("the log does not say what happened: %s", a.logs.String())
	}

	// A model that cannot be reached, with a hint that fits the server.
	a2 := newAsst(t, asstOptions{flags: map[string]string{"ai": "true", "ai-provider": "openai", "ai-model": "llama3.2", "ai-base-url": "http://127.0.0.1:1/v1"}})
	sid = a2.session("practice", "quiz")
	_, events, _ = a2.stream(sid, "a1", `{"message":"x"}`)
	e = events[len(events)-1].Data
	if e["code"] != "unreachable" || e["message"] != "Could not reach 127.0.0.1." || e["retryable"] != true {
		t.Errorf("unreachable: %v", e)
	}
}

func TestSettingsAPI(t *testing.T) {
	a := newAsst(t, asstOptions{flags: map[string]string{"ai-provider": "openai"}, env: map[string]string{}})
	sid := a.session("practice", "quiz")

	v := a.json("GET", "/api/settings", "", 200)
	as := v["assistant"].(map[string]any)
	if v["writable"] != true || v["hosted"] != false || as["configured"] != false || as["problem"] != "The assistant is switched off." {
		t.Errorf("fresh settings = %v", v)
	}
	wantField := func(name string, got any, value any, source string, locked bool, by string) {
		t.Helper()
		f := got.(map[string]any)
		if f["value"] != value || f["source"] != source || f["locked"] != locked || (by != "" && f["lockedBy"] != by) || (by == "" && f["lockedBy"] != nil) {
			t.Errorf("%s = %v", name, f)
		}
	}
	wantField("enabled", as["enabled"], false, "default", false, "")
	wantField("provider", as["provider"], "openai", "flag", true, "--ai-provider")
	wantField("model", as["model"], "", "default", false, "")
	wantField("baseURL", as["baseURL"], "https://api.openai.com/v1", "default", false, "")
	if k := as["apiKey"].(map[string]any); k["set"] != false || k["source"] != "default" || k["locked"] != false || k["last4"] != nil {
		t.Errorf("apiKey = %v", k)
	}
	ps := v["providers"].([]any)
	if len(ps) != 2 || ps[0].(map[string]any)["id"] != "anthropic" || ps[0].(map[string]any)["defaultModel"] != "claude-opus-5-5" ||
		ps[0].(map[string]any)["needsKey"] != true || len(ps[1].(map[string]any)["presets"].([]any)) != 4 {
		t.Errorf("providers = %v", ps)
	}
	if tc := v["traceContent"].(map[string]any); tc["value"] != false || tc["reason"] != "no local observability stack" {
		t.Errorf("traceContent = %v", tc)
	}

	// A change is saved, answered in the shape of GET, and live at once.
	v = a.json("PUT", "/api/settings", `{"assistant":{"enabled":true,"model":"fake","baseURL":"`+a.fakeAt+`"}}`, 200)
	as = v["assistant"].(map[string]any)
	if as["configured"] != true || as["problem"] != "" {
		t.Fatalf("after PUT: %v", as)
	}
	wantField("enabled", as["enabled"], true, "file", false, "")
	wantField("model", as["model"], "fake", "file", false, "")
	wantField("baseURL", as["baseURL"], a.fakeAt, "file", false, "")
	_, events, _ := a.stream(sid, "a1", `{"message":"x"}`)
	if events[0].Data["model"] != "fake" {
		t.Errorf("the change is not live: %v", events[0].Data)
	}
	a.json("PUT", "/api/settings", `{"assistant":{"model":"other"}}`, 200)
	_, events, _ = a.stream(sid, "a1", `{"message":"y"}`)
	if events[0].Data["model"] != "other" {
		t.Errorf("the second change is not live: %v", events[0].Data)
	}
	if cfg := a.json("GET", "/api/config", "", 200); cfg["tutor"] != true || cfg["assistant"].(map[string]any)["model"] != "other" ||
		cfg["assistant"].(map[string]any)["provider"] != "openai_compatible" {
		t.Errorf("config = %v", cfg)
	}

	// Refusals: a locked field is 409, an invalid value 400, both with the field.
	for name, tc := range map[string]struct {
		body   string
		status int
		field  string
	}{
		"locked provider":     {`{"assistant":{"provider":"anthropic"}}`, 409, "provider"},
		"unknown provider":    {`{"assistant":{"provider":"gemini"}}`, 409, "provider"},
		"metadata address":    {`{"assistant":{"baseURL":"http://169.254.169.254/v1"}}`, 400, "baseURL"},
		"public plain http":   {`{"assistant":{"baseURL":"http://93.184.216.34/v1"}}`, 400, "baseURL"},
		"credentials in URL":  {`{"assistant":{"baseURL":"https://user:pw@example.com/v1"}}`, 400, "baseURL"},
		"not a URL":           {`{"assistant":{"baseURL":"localhost:11434"}}`, 400, "baseURL"},
		"white space in key":  {`{"assistant":{"apiKey":"a key"}}`, 400, "apiKey"},
		"a model too long":    {`{"assistant":{"model":"` + strings.Repeat("m", 201) + `"}}`, 400, "model"},
		"not JSON":            {`{"assistant":`, 400, ""},
		"no body":             {``, 415, ""},
		"a body over 16 KiB":  {`{"assistant":{"model":"` + strings.Repeat("m", 17000) + `"}}`, 400, ""},
		"enabled of a type":   {`{"assistant":{"enabled":"yes"}}`, 400, ""},
		"repeats locked, ok":  {`{"assistant":{"provider":"openai","model":"other"}}`, 200, ""},
		"an empty change, ok": {`{}`, 200, ""},
	} {
		res, raw := a.do("PUT", "/api/settings", tc.body)
		if res.StatusCode != tc.status {
			t.Errorf("%s = %d %s, want %d", name, res.StatusCode, raw, tc.status)
			continue
		}
		if tc.field != "" && !strings.Contains(raw, `"field":"`+tc.field+`"`) {
			t.Errorf("%s: no field in %s", name, raw)
		}
	}
	if got := a.json("GET", "/api/settings", "", 200)["assistant"].(map[string]any)["model"].(map[string]any)["value"]; got != "other" {
		t.Errorf("a refused change changed the model to %v", got)
	}

	// The test endpoint answers 200 for a test that ran, whatever it found.
	ok := a.json("POST", "/api/settings/test", `{"assistant":{"model":"probe"}}`, 200)
	if ok["ok"] != true || ok["provider"] != "openai_compatible" || ok["model"] != "probe" || ok["latencyMs"] == nil {
		t.Errorf("test = %v", ok)
	}
	bad := a.json("POST", "/api/settings/test", `{"assistant":{"baseURL":"http://127.0.0.1:1/v1"}}`, 200)
	if bad["ok"] != false || bad["code"] != "unreachable" || bad["message"] != "Could not reach 127.0.0.1." {
		t.Errorf("failing test = %v", bad)
	}
	if res, raw := a.do("POST", "/api/settings/test", `{"assistant":{"provider":"anthropic"}}`); res.StatusCode != 409 || !strings.Contains(raw, `"field":"provider"`) {
		t.Errorf("test of a locked field = %d %s", res.StatusCode, raw)
	}
	// The test saved nothing.
	if got := a.json("GET", "/api/settings", "", 200)["assistant"].(map[string]any)["model"].(map[string]any)["value"]; got != "other" {
		t.Errorf("a test saved the model %v", got)
	}

	// The settings file: private, and outside the database.
	info, err := os.Stat(filepath.Join(a.dir, settings.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("settings.json has mode %o", info.Mode().Perm())
	}
}

// A hosted server is configured by its deployment. The API shows the
// configuration and changes nothing.
func TestSettingsOnAHostedServer(t *testing.T) {
	fake := &aitest.Fake{RequireKey: secretKey}
	upstream := httptest.NewServer(fake)
	defer upstream.Close()
	a := newAsst(t, asstOptions{hosted: true, flags: map[string]string{}, env: map[string]string{
		"KUBEDOJO_AI": "true", "KUBEDOJO_AI_PROVIDER": "openai", "KUBEDOJO_AI_MODEL": "fake",
		"KUBEDOJO_AI_BASE_URL": upstream.URL + "/v1", "KUBEDOJO_AI_API_KEY": secretKey}})
	v := a.json("GET", "/api/settings", "", 200)
	as := v["assistant"].(map[string]any)
	if v["writable"] != false || v["hosted"] != true || as["configured"] != true {
		t.Errorf("settings = %v", v)
	}
	k := as["apiKey"].(map[string]any)
	if k["set"] != true || k["last4"] != nil || k["source"] != "env" || k["locked"] != true || k["lockedBy"] != "KUBEDOJO_AI_API_KEY" {
		t.Errorf("apiKey on a hosted server = %v", k)
	}
	for _, f := range []string{"enabled", "provider", "model", "baseURL"} {
		if as[f].(map[string]any)["locked"] != true {
			t.Errorf("%s is not locked: %v", f, as[f])
		}
	}
	for _, req := range [][2]string{{"PUT", "/api/settings"}, {"POST", "/api/settings/test"}} {
		res, raw := a.do(req[0], req[1], `{"assistant":{"model":"other"}}`)
		if res.StatusCode != 403 || !strings.Contains(raw, `"code":"hosted"`) {
			t.Errorf("%s %s = %d %s, want 403", req[0], req[1], res.StatusCode, raw)
		}
	}
	if cfg := a.json("GET", "/api/config", "", 200); cfg["assistant"].(map[string]any)["writable"] != false || cfg["tutor"] != true {
		t.Errorf("config = %v", cfg)
	}
	// It works, with the key of the environment.
	sid := a.session("practice", "quiz")
	if _, events, _ := a.stream(sid, "a1", `{"message":"x"}`); events[len(events)-1].Name != "done" {
		t.Errorf("chat on a hosted server: %s", names(events))
	}
	if _, err := os.Stat(filepath.Join(a.dir, settings.FileName)); err == nil {
		t.Error("a hosted server wrote settings.json")
	}
	a.noKeyAnywhere(secretKey)
}

// noKeyAnywhere fails if the key is in any response body read so far or in
// the log.
func (a *asst) noKeyAnywhere(key string) {
	a.t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, b := range a.bodies {
		if strings.Contains(b, key) {
			a.t.Errorf("the key is in a response: %s", b)
		}
	}
	if strings.Contains(a.logs.String(), key) {
		a.t.Errorf("the key is in the log: %s", a.logs.String())
	}
	if len(a.bodies) == 0 || a.logs.String() == "" {
		a.t.Error("nothing was checked: no responses or no log lines")
	}
}

// A key written through the settings API is write-only: no response, no log
// line and no error carries it afterwards.
func TestKeyIsWriteOnly(t *testing.T) {
	fake := &aitest.Fake{RequireKey: secretKey}
	a := newAsst(t, asstOptions{fake: fake, flags: map[string]string{}, env: map[string]string{}})
	sid := a.session("practice", "quiz")

	v := a.json("PUT", "/api/settings", `{"assistant":{"enabled":true,"provider":"openai","model":"fake","baseURL":"`+a.fakeAt+`","apiKey":"`+secretKey+`"}}`, 200)
	k := v["assistant"].(map[string]any)["apiKey"].(map[string]any)
	if k["set"] != true || k["last4"] != "WXYZ" || k["source"] != "file" || k["locked"] != false {
		t.Errorf("apiKey after PUT = %v", k)
	}
	// The key works: the fake server only answers to it.
	if _, events, _ := a.stream(sid, "a1", `{"message":"x"}`); events[len(events)-1].Name != "done" {
		t.Fatalf("chat with the stored key: %s", names(events))
	}
	if got := fake.Requests()[0].Authorization; got != "Bearer "+secretKey {
		t.Fatal("the stored key was not sent to the provider")
	}

	// Everything that could echo it.
	a.json("GET", "/api/settings", "", 200)
	a.json("GET", "/api/config", "", 200)
	a.json("GET", "/api/sessions/"+sid+"/questions/a1/assistant", "", 200)
	a.json("POST", "/api/settings/test", `{}`, 200)
	a.json("POST", "/api/settings/test", `{"assistant":{"apiKey":"`+secretKey+`"}}`, 200)
	a.json("PUT", "/api/settings", `{"assistant":{"model":"fake2"}}`, 200)
	a.do("PUT", "/api/settings", `{"assistant":{"apiKey":"`+secretKey+`","baseURL":"http://169.254.169.254/"}}`)
	a.do("PUT", "/api/settings", `{"assistant":{"apiKey":"`+secretKey+` "}}`)
	a.do("PUT", "/api/settings", `{"assistant":{"apiKey":"`+secretKey+`","provider":"gemini"}}`)
	a.do("PUT", "/api/settings", `{"assistant":{"apiKey":"`+secretKey+`"`)
	a.do("POST", "/api/sessions/"+sid+"/questions/a1/tutor", `{"ask":"x"}`)
	// A provider that rejects the key and quotes it in its answer.
	strict := httptest.NewServer(&aitest.Fake{RequireKey: "another-key"})
	defer strict.Close()
	a.json("PUT", "/api/settings", `{"assistant":{"baseURL":"`+strict.URL+`/v1","apiKey":"`+secretKey+`"}}`, 200)
	_, events, _ := a.stream(sid, "a1", `{"message":"y"}`)
	if events[len(events)-1].Data["code"] != "auth" {
		t.Errorf("rejected key: %s", names(events))
	}
	if bad := a.json("POST", "/api/settings/test", `{}`, 200); bad["code"] != "auth" {
		t.Errorf("test with a rejected key: %v", bad)
	}
	a.do("POST", "/api/sessions/"+sid+"/questions/a1/tutor", `{"ask":"x"}`)
	// The key moves to nowhere: another server without a new key drops it.
	v = a.json("PUT", "/api/settings", `{"assistant":{"baseURL":"http://127.0.0.1:1/v1"}}`, 200)
	if v["assistant"].(map[string]any)["apiKey"].(map[string]any)["set"] != false {
		t.Error("the key followed to another server")
	}
	a.noKeyAnywhere(secretKey)

	// It was in the settings file while it was set, and nowhere else on disk
	// that a backup copies: the file is not part of the database.
	a.json("PUT", "/api/settings", `{"assistant":{"baseURL":"`+a.fakeAt+`","apiKey":"`+secretKey+`"}}`, 200)
	raw, err := os.ReadFile(filepath.Join(a.dir, settings.FileName))
	if err != nil || !strings.Contains(string(raw), secretKey) {
		t.Errorf("the key is not in settings.json: %v", err)
	}
	a.json("PUT", "/api/settings", `{"assistant":{"apiKey":""}}`, 200)
	if raw, _ = os.ReadFile(filepath.Join(a.dir, settings.FileName)); strings.Contains(string(raw), secretKey) {
		t.Error("a removed key is still in settings.json")
	}
}
