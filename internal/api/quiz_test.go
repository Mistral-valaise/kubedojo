package api_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Mistral-valaise/kubedojo/internal/api"
	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox/sandboxtest"
	"github.com/Mistral-valaise/kubedojo/internal/session"
	"github.com/Mistral-valaise/kubedojo/internal/store"
)

// Texts that exist only in the answer key. None of them may reach a client
// before the question is revealed.
const (
	keyExplanation = "KEY-EXPLANATION"
	keyWhyWrong    = "KEY-WHY-WRONG"
	keyWhyRight    = "KEY-WHY-RIGHT"
)

func quizQuestion(domain string) string {
	return `
title: Single
domain: ` + domain + `
stem: Pick the right one on {{host}}.
options:
  - {text: alpha, correct: true, explain: ` + keyWhyRight + `}
  - {text: beta, explain: ` + keyWhyWrong + `}
  - {text: gamma}
  - {text: delta}
explanation: ` + keyExplanation + `
references: [{title: Docs, url: "https://kubernetes.io/docs/"}]
`
}

type quizAPI struct {
	t     *testing.T
	url   string
	track *catalog.Track
}

func newQuizAPI(t *testing.T, extra fstest.MapFS) *quizAPI {
	t.Helper()
	f := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }
	fsys := fstest.MapFS{
		"quiz/track.yaml": f("title: Quiz\nshort: QZ\nkind: theory\ndocs: [{title: Curriculum, url: \"https://example.org/c\"}]\n" +
			"exam: {durationMinutes: 10, passPercent: 75, questions: 2}\n" +
			"domains: [{id: a, title: A, weight: 50}, {id: b, title: B, weight: 50}]\n"),
		"quiz/lessons/a.md":      f("# A"),
		"quiz/lessons/b.md":      f("# B"),
		"quiz/questions/a1.yaml": f(quizQuestion("a")),
		"quiz/questions/b1.yaml": f(quizQuestion("b")),
	}
	for name, file := range extra {
		fsys[name] = file
	}
	cat, err := catalog.Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	mgr, err := session.NewManager(session.Config{Catalog: cat, Store: st, Driver: &sandboxtest.Driver{}})
	if err != nil {
		t.Fatal(err)
	}
	s := &api.Server{Catalog: cat, Sessions: mgr, Store: st, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(func() {
		srv.Close()
		mgr.Close()
		st.Close()
	})
	return &quizAPI{t: t, url: srv.URL, track: cat.Tracks["quiz"]}
}

// call sends a request and returns the status and the raw body.
func (a *quizAPI) call(method, path, body string) (int, string) {
	a.t.Helper()
	req, err := http.NewRequest(method, a.url+path, strings.NewReader(body))
	if err != nil {
		a.t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(raw)
}

// ok is call for requests that must succeed.
func (a *quizAPI) ok(method, path, body string) string {
	a.t.Helper()
	code, raw := a.call(method, path, body)
	if code < 200 || code > 299 {
		a.t.Fatalf("%s %s = %d %s", method, path, code, raw)
	}
	return raw
}

func (a *quizAPI) session(mode string) string {
	a.t.Helper()
	var v struct{ ID string }
	if err := json.Unmarshal([]byte(a.ok("POST", "/api/sessions", `{"mode":"`+mode+`","track":"quiz"}`)), &v); err != nil || v.ID == "" {
		a.t.Fatalf("no session id: %v", err)
	}
	return v.ID
}

type choiceJSON struct {
	Multiple bool
	Options  []struct{ ID, Text string }
	Answer   []string
	Review   *struct {
		Correct     []string
		Explanation string
		Options     map[string]string
		References  []catalog.Link
	}
}

func (a *quizAPI) question(sid, qid string) (string, choiceJSON) {
	a.t.Helper()
	raw := a.ok("GET", "/api/sessions/"+sid+"/questions/"+qid, "")
	var v struct {
		Task   string
		Choice *choiceJSON
	}
	if err := json.Unmarshal([]byte(raw), &v); err != nil || v.Choice == nil {
		a.t.Fatalf("question without choice: %v %s", err, raw)
	}
	if strings.Contains(v.Task, "{{host}}") || !strings.Contains(v.Task, "Pick the right one") {
		a.t.Errorf("task = %q, want the rendered stem", v.Task)
	}
	return raw, *v.Choice
}

func (a *quizAPI) correct(qid string) string { return a.track.Question(qid).Choice.Correct()[0] }

func (a *quizAPI) wrong(qid string) string {
	for _, o := range a.track.Question(qid).Choice.Options {
		if !o.Correct {
			return o.ID
		}
	}
	return ""
}

// sealed fails if raw carries any part of the answer key.
func (a *quizAPI) sealed(what, raw string) {
	a.t.Helper()
	for _, secret := range []string{`"review"`, `"correct"`, `"Correct"`, `"explain`, `"Explain`, `"Choice"`, keyExplanation, keyWhyWrong, keyWhyRight} {
		if strings.Contains(raw, secret) {
			a.t.Errorf("%s leaks %s: %s", what, secret, raw)
		}
	}
}

func TestQuizTrackViewHidesTheAnswerKey(t *testing.T) {
	a := newQuizAPI(t, nil)
	for _, path := range []string{"/api/tracks", "/api/tracks/quiz", "/api/tracks/quiz/progress", "/api/tracks/quiz/lessons/a"} {
		raw := a.ok("GET", path, "")
		a.sealed(path, raw)
		// A track lists questions, not their options.
		for _, o := range a.track.Question("a1").Choice.Options {
			if strings.Contains(raw, o.ID) || strings.Contains(raw, `"`+o.Text+`"`) {
				t.Errorf("%s contains option %s (%s)", path, o.ID, o.Text)
			}
		}
	}
	var tr struct {
		Kind         string
		Docs         []catalog.Link
		Environments map[string]struct{ Type string }
		Questions    []struct{ ID, Type string }
	}
	raw := a.ok("GET", "/api/tracks/quiz", "")
	if err := json.Unmarshal([]byte(raw), &tr); err != nil {
		t.Fatal(err)
	}
	if tr.Kind != catalog.KindTheory || len(tr.Docs) != 1 || tr.Docs[0].URL != "https://example.org/c" {
		t.Errorf("kind %q, docs %+v", tr.Kind, tr.Docs)
	}
	if !strings.Contains(raw, `"environments":{}`) {
		t.Errorf("a track without environments must serialise them as {}: %s", raw)
	}
	if len(tr.Questions) != 2 || tr.Questions[0].Type != catalog.TypeChoice {
		t.Errorf("questions = %+v", tr.Questions)
	}
}

func TestQuizPracticeOverHTTP(t *testing.T) {
	a := newQuizAPI(t, nil)
	sid := a.session("practice")
	q := "/api/sessions/" + sid + "/questions/a1"

	// Opening a question is enough: no prepare call, nothing to wait for.
	raw, c := a.question(sid, "a1")
	a.sealed("a fresh question", raw)
	if c.Multiple || len(c.Options) != 4 || c.Answer == nil || len(c.Answer) != 0 || c.Review != nil {
		t.Fatalf("fresh question: %+v", c)
	}
	_, again := a.question(sid, "a1")
	if !slices.Equal(again.Options, c.Options) {
		t.Error("the options changed their order between two reads")
	}

	// An answer can be saved without grading it; it reveals nothing.
	a.ok("PATCH", q, `{"answer":["`+a.wrong("a1")+`"]}`)
	raw, c = a.question(sid, "a1")
	a.sealed("an answered question", raw)
	if !slices.Equal(c.Answer, []string{a.wrong("a1")}) {
		t.Errorf("answer = %v", c.Answer)
	}
	a.sealed("the session of an answered question", a.ok("GET", "/api/sessions/"+sid, ""))

	for name, body := range map[string]string{
		"unknown option": `{"answer":["nope00"]}`,
		"two options":    `{"answer":["` + a.correct("a1") + `","` + a.wrong("a1") + `"]}`,
	} {
		if code, raw := a.call("PATCH", q, body); code != 400 {
			t.Errorf("PATCH with %s = %d %s, want 400", name, code, raw)
		}
		if code, raw := a.call("POST", q+"/check", body); code != 400 {
			t.Errorf("check with %s = %d %s, want 400", name, code, raw)
		}
	}
	if code, _ := a.call("GET", q+"/hints/1", ""); code != 404 {
		t.Errorf("hint of a choice question = %d, want 404", code)
	}

	// Submit a wrong answer: graded, and now the review is there.
	var res struct {
		Score, Max int
		Passed     bool
		Checks     []any
	}
	if err := json.Unmarshal([]byte(a.ok("POST", q+"/check", `{"answer":["`+a.wrong("a1")+`"]}`)), &res); err != nil {
		t.Fatal(err)
	}
	if res.Passed || res.Score != 0 || res.Max != 1 || res.Checks == nil {
		t.Errorf("check = %+v, want 0/1 and an empty list of checks", res)
	}
	_, c = a.question(sid, "a1")
	if c.Review == nil {
		t.Fatal("no review after check")
	}
	r := c.Review
	if !slices.Equal(r.Correct, []string{a.correct("a1")}) || r.Explanation != keyExplanation ||
		r.Options[a.wrong("a1")] != keyWhyWrong || r.Options[a.correct("a1")] != keyWhyRight || len(r.Options) != 2 ||
		len(r.References) != 1 || r.References[0].URL != "https://kubernetes.io/docs/" {
		t.Errorf("review = %+v", r)
	}
	if code, _ := a.call("POST", q+"/check", `{"answer":["`+a.correct("a1")+`"]}`); code != 409 {
		t.Errorf("answering a revealed question = %d, want 409", code)
	}
	// The other question is still sealed.
	raw, _ = a.question(sid, "b1")
	a.sealed("another question", raw)

	// Reset, then the right answer.
	a.ok("POST", q+"/prepare?reset=1", "")
	raw, c = a.question(sid, "a1")
	a.sealed("a reset question", raw)
	if len(c.Answer) != 0 {
		t.Errorf("answer after reset = %v", c.Answer)
	}
	a.ok("POST", q+"/check", `{"answer":["`+a.correct("a1")+`"]}`)

	// Opening the solution reveals, as it does for tasks.
	var sol struct {
		Markdown string
		Review   *struct{ Correct []string }
	}
	if err := json.Unmarshal([]byte(a.ok("GET", "/api/sessions/"+sid+"/questions/b1/solution", "")), &sol); err != nil {
		t.Fatal(err)
	}
	if sol.Review == nil || !slices.Equal(sol.Review.Correct, []string{a.correct("b1")}) || !strings.Contains(sol.Markdown, keyExplanation) || !strings.Contains(sol.Markdown, "alpha") {
		t.Errorf("solution = %+v", sol)
	}
	if _, c := a.question(sid, "b1"); c.Review == nil {
		t.Error("no review after the solution was opened")
	}
	if code, _ := a.call("PATCH", "/api/sessions/"+sid+"/questions/b1", `{"answer":["`+a.correct("b1")+`"]}`); code != 409 {
		t.Errorf("answering after the solution was opened = %d, want 409", code)
	}
}

// Readiness follows the latest attempt at a choice question, not the best.
func TestQuizProgressUsesTheLatestAttempt(t *testing.T) {
	a := newQuizAPI(t, nil)
	sid := a.session("practice")
	q := "/api/sessions/" + sid + "/questions/a1"
	progress := func() (solved int, readiness, best, latest float64) {
		var p struct {
			Readiness float64
			Domains   []struct {
				ID     string
				Solved int
			}
			Questions map[string]struct{ Best, Latest float64 }
		}
		if err := json.Unmarshal([]byte(a.ok("GET", "/api/tracks/quiz/progress", "")), &p); err != nil {
			t.Fatal(err)
		}
		return p.Domains[0].Solved, p.Readiness, p.Questions["a1"].Best, p.Questions["a1"].Latest
	}
	a.ok("POST", q+"/check", `{"answer":["`+a.correct("a1")+`"]}`)
	if solved, readiness, best, latest := progress(); solved != 1 || readiness != 50 || best != 1 || latest != 1 {
		t.Errorf("after a right answer: solved %d, readiness %v, best %v, latest %v", solved, readiness, best, latest)
	}
	a.ok("POST", q+"/prepare?reset=1", "")
	a.ok("POST", q+"/check", `{"answer":["`+a.wrong("a1")+`"]}`)
	if solved, readiness, best, latest := progress(); solved != 0 || readiness != 0 || best != 1 || latest != 0 {
		t.Errorf("after a wrong answer: solved %d, readiness %v, best %v, latest %v", solved, readiness, best, latest)
	}
}

func TestQuizExamOverHTTP(t *testing.T) {
	a := newQuizAPI(t, nil)
	raw := a.ok("POST", "/api/sessions", `{"mode":"exam","track":"quiz"}`)
	var ses struct {
		ID, Status string
		Questions  []struct{ ID string }
	}
	if err := json.Unmarshal([]byte(raw), &ses); err != nil {
		t.Fatal(err)
	}
	if ses.Status != session.StatusReady || len(ses.Questions) != 2 {
		t.Fatalf("new exam = %s", raw)
	}
	a.sealed("a new exam", raw)
	base := "/api/sessions/" + ses.ID
	if code, _ := a.call("GET", base+"/questions/a1", ""); code != 409 {
		t.Errorf("question before the start = %d, want 409", code)
	}
	a.sealed("the started exam", a.ok("POST", base+"/start", ""))

	// Autosave: one right, one wrong. Every response stays sealed.
	a.sealed("PATCH", a.ok("PATCH", base+"/questions/a1", `{"answer":["`+a.correct("a1")+`"],"flagged":true}`))
	a.ok("PATCH", base+"/questions/b1", `{"answer":["`+a.wrong("b1")+`"]}`)
	for _, qid := range []string{"a1", "b1"} {
		raw, c := a.question(ses.ID, qid)
		a.sealed("question "+qid+" during the exam", raw)
		if len(c.Answer) != 1 || len(c.Options) != 4 || strings.Contains(raw, `"last"`) {
			t.Errorf("question %s during the exam: %s", qid, raw)
		}
	}
	raw = a.ok("GET", base, "")
	a.sealed("the running exam", raw)
	if strings.Contains(raw, `"last"`) || strings.Contains(raw, `"report"`) || !strings.Contains(raw, `"flagged":true`) {
		t.Errorf("running exam: %s", raw)
	}
	a.sealed("the session list", a.ok("GET", "/api/sessions", ""))
	for _, path := range []string{"/questions/a1/check", "/questions/a1/tutor"} {
		if code, raw := a.call("POST", base+path, ""); code != 409 && code != 404 {
			t.Errorf("POST %s during the exam = %d %s", path, code, raw)
		} else {
			a.sealed(path, raw)
		}
	}
	code, raw := a.call("GET", base+"/questions/a1/solution", "")
	if code != 409 {
		t.Errorf("solution during the exam = %d, want 409", code)
	}
	a.sealed("the refused solution", raw)

	// Finish answers with the finished session: no polling for a grade.
	raw = a.ok("POST", base+"/finish", "")
	var done struct {
		Status string
		Report *session.Report
	}
	if err := json.Unmarshal([]byte(raw), &done); err != nil {
		t.Fatal(err)
	}
	if done.Status != session.StatusFinished || done.Report == nil {
		t.Fatalf("finish = %s", raw)
	}
	if r := done.Report; r.Score != 1 || r.Max != 2 || r.Percent != 50 || r.Passed || len(r.Domains) != 2 {
		t.Errorf("report = %+v", r)
	}
	for qid, passed := range map[string]bool{"a1": true, "b1": false} {
		raw, c := a.question(ses.ID, qid)
		if c.Review == nil || !slices.Equal(c.Review.Correct, []string{a.correct(qid)}) || c.Review.Explanation != keyExplanation {
			t.Errorf("question %s after the exam has no review: %s", qid, raw)
		}
		var v struct {
			State struct{ Last *struct{ Passed bool } }
		}
		_ = json.Unmarshal([]byte(raw), &v)
		if v.State.Last == nil || v.State.Last.Passed != passed {
			t.Errorf("question %s after the exam: last = %+v, want passed=%v", qid, v.State.Last, passed)
		}
	}
	if code, _ := a.call("PATCH", base+"/questions/a1", `{"answer":[]}`); code != 409 {
		t.Errorf("answering after the exam = %d, want 409", code)
	}
}

func TestPath(t *testing.T) {
	if code, _ := newQuizAPI(t, nil).call("GET", "/api/path", ""); code != 404 {
		t.Errorf("content without a path: %d, want 404", code)
	}
	a := newQuizAPI(t, fstest.MapFS{"path.yaml": {Data: []byte("id: p\ntitle: The path\nsteps: [{track: quiz, role: required, why: because}]\n")}})
	var p catalog.Path
	if err := json.Unmarshal([]byte(a.ok("GET", "/api/path", "")), &p); err != nil {
		t.Fatal(err)
	}
	if p.Title != "The path" || len(p.Steps) != 1 || p.Steps[0].Track != "quiz" || p.Steps[0].Role != catalog.RoleRequired {
		t.Errorf("path = %+v", p)
	}
}
