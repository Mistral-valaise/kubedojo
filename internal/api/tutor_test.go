package api_test

import (
	"strings"
	"testing"

	"github.com/Mistral-valaise/kubedojo/content"
	"github.com/Mistral-valaise/kubedojo/internal/ai/aitest"
	"github.com/Mistral-valaise/kubedojo/internal/assistant"
	"github.com/Mistral-valaise/kubedojo/internal/catalog"
)

// The assistant's snapshot runs on the main machine of a question's
// environment. On a Linux host there is no kubectl, and a snapshot full of
// "command not found" would send the model looking for a cluster problem.
func TestTutorSnapshotFitsTheEnvironment(t *testing.T) {
	cat, err := catalog.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[bool]bool{}
	for _, tr := range cat.List() {
		for _, q := range tr.Questions {
			if q.Type != catalog.TypeTask {
				continue
			}
			host := tr.NeedsContainer()
			seen[host] = true
			if got := strings.Contains(assistant.SnapshotScript(cat, q), "kubectl"); got == host {
				t.Fatalf("%s/%s: snapshot uses kubectl = %v on a host = %v", tr.ID, q.ID, got, host)
			}
		}
	}
	if !seen[true] || !seen[false] {
		t.Errorf("the shipped content no longer has both environment types: %v", seen)
	}
}

// POST .../tutor is what the UI of 0.4 calls. It answers as it always did,
// {"markdown": "..."}, now on top of the assistant, and says it is deprecated.
func TestTutorEndpointStillAnswersAsBefore(t *testing.T) {
	a := newAsst(t, asstOptions{})
	sid := a.session("practice", "quiz")
	path := "/api/sessions/" + sid + "/questions/a1/tutor"

	res, raw := a.do("POST", path, `{"ask":"Why is it alpha?"}`)
	if res.StatusCode != 200 {
		t.Fatalf("POST tutor = %d %s", res.StatusCode, raw)
	}
	if res.Header.Get("Deprecation") != "true" || !strings.Contains(res.Header.Get("Link"), "/questions/a1/assistant") {
		t.Errorf("headers = %v", res.Header)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	v := a.json("POST", path, `{"ask":"Why is it alpha?"}`, 200)
	if len(v) != 1 || v["markdown"] != aitest.CannedAnswer("Why is it alpha?") {
		t.Errorf("body = %v", v)
	}
	// An empty body asks the default question, as before.
	req := a.fake.Requests()
	if v = a.json("POST", path, ``, 200); v["markdown"] == "" {
		t.Errorf("no body: %v", v)
	}
	if last := a.fake.Requests()[len(req)].LastUser(); !strings.HasSuffix(last, "What went wrong, and what should I look at next?") {
		t.Errorf("the default question was not asked: %q", last)
	}
	// The same checks, the same context as the assistant: it is one service.
	if last := a.fake.Requests()[0].LastUser(); !strings.HasPrefix(last, "<context>") {
		t.Errorf("the tutor's question went out without context: %q", last)
	}
	// What was asked here shows up in the assistant's conversation.
	if n := len(a.json("GET", "/api/sessions/"+sid+"/questions/a1/assistant", "", 200)["messages"].([]any)); n != 6 {
		t.Errorf("%d messages in the conversation, want 6", n)
	}
	// The flag old clients look at.
	if cfg := a.json("GET", "/api/config", "", 200); cfg["tutor"] != true {
		t.Errorf("config.tutor = %v", cfg["tutor"])
	}
}

func TestTutorEndpointErrorsAsBefore(t *testing.T) {
	// Not configured: 404 with the old sentence.
	off := newAsst(t, asstOptions{flags: map[string]string{}})
	sid := off.session("practice", "quiz")
	res, raw := off.do("POST", "/api/sessions/"+sid+"/questions/a1/tutor", `{"ask":"x"}`)
	if res.StatusCode != 404 || !strings.Contains(raw, "The AI tutor is not configured.") || res.Header.Get("Deprecation") != "true" {
		t.Errorf("not configured = %d %s", res.StatusCode, raw)
	}

	// A running exam: 409 with the old sentence.
	a := newAsst(t, asstOptions{})
	exam := a.session("exam", "quiz")
	a.json("POST", "/api/sessions/"+exam+"/start", "", 200)
	if v := a.json("POST", "/api/sessions/"+exam+"/questions/a1/tutor", `{"ask":"x"}`, 409); v["error"] != "The tutor is available after the exam." {
		t.Errorf("during an exam: %v", v)
	}
	// Bad JSON, an unknown question.
	sid = a.session("practice", "quiz")
	a.json("POST", "/api/sessions/"+sid+"/questions/a1/tutor", `{"ask":`, 400)
	a.json("POST", "/api/sessions/"+sid+"/questions/nope/tutor", `{"ask":"x"}`, 404)

	// The model fails: 502, with our words and not the provider's.
	broken := newAsst(t, asstOptions{fake: &aitest.Fake{Status: 500}})
	sid = broken.session("practice", "quiz")
	v := broken.json("POST", "/api/sessions/"+sid+"/questions/a1/tutor", `{"ask":"x"}`, 502)
	if v["error"] != "The tutor could not answer: The provider answered with an error (HTTP 500)." {
		t.Errorf("upstream failure: %v", v)
	}
}
