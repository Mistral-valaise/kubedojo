package main

import (
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox"
	"github.com/Mistral-valaise/kubedojo/internal/session"
	"github.com/Mistral-valaise/kubedojo/internal/store"
)

const proofSingle = `
title: Single
domain: d
stem: Pick one.
options:
  - {text: right, correct: true}
  - {text: wrong one}
  - {text: wrong two}
  - {text: wrong three}
explanation: because
references: [{title: Docs, url: "https://kubernetes.io/docs/"}]
`

const proofMultiple = `
title: Multiple
domain: d
multiple: true
stem: Pick two.
options:
  - {text: right one, correct: true}
  - {text: right two, correct: true}
  - {text: wrong one}
  - {text: wrong two}
explanation: because
references: [{title: Docs, url: "https://kubernetes.io/docs/"}]
`

// The quiz proofs run against the kind driver without it ever being called:
// the test has no Docker to offer.
func TestQuizProofs(t *testing.T) {
	file := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }
	cat, err := catalog.Load(fstest.MapFS{
		"quiz/track.yaml":        file("title: Quiz\nkind: theory\nexam: {durationMinutes: 10, passPercent: 75, questions: 2}\ndomains: [{id: d, title: D, weight: 100}]\n"),
		"quiz/lessons/d.md":      file("# D"),
		"quiz/questions/s.yaml":  file(proofSingle),
		"quiz/questions/m.yaml":  file(proofMultiple),
		"lab/track.yaml":         file("title: Lab\nkind: skills\nexam: {durationMinutes: 10, passPercent: 50, questions: 1}\ndomains: [{id: d, title: D, weight: 100}]\nenvironments: {base: {type: container}}\n"),
		"lab/lessons/d.md":       file("# D"),
		"other/track.yaml":       file("title: Other\nkind: theory\nexam: {durationMinutes: 10, passPercent: 75, questions: 1}\ndomains: [{id: d, title: D, weight: 100}]\n"),
		"other/lessons/d.md":     file("# D"),
		"other/questions/s.yaml": file(proofSingle),
	})
	if err != nil {
		t.Fatal(err)
	}
	for only, want := range map[string]bool{"quiz": false, "other": false, "lab": true, "": true} {
		if got := needsSandbox(cat, only); got != want {
			t.Errorf("needsSandbox(%q) = %v, want %v", only, got, want)
		}
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	mgr, err := session.NewManager(session.Config{Catalog: cat, Store: st, Driver: &sandbox.Kind{}})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	tr := cat.Tracks["quiz"]
	ses, err := mgr.Practice(tr.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	// no answer, the correct answer, each option alone; for the multiple
	// choice also the correct answer plus each wrong option.
	for id, runs := range map[string]int{"s": 2 + 4, "m": 2 + 4 + 2} {
		qp := proveChoice(mgr, ses.ID, tr.Question(id))
		if qp.Status != statusOK || len(qp.Runs) != runs {
			t.Errorf("%s: status %s (%s) after %d runs, want ok after %d", id, qp.Status, qp.Reason, len(qp.Runs), runs)
		}
		passes := 0
		for _, r := range qp.Runs {
			if r.After != nil && r.After.Passed {
				passes++
			}
		}
		// A single choice passes for the correct answer and for that option alone.
		if want := map[string]int{"s": 2, "m": 1}[id]; passes != want {
			t.Errorf("%s: %d runs scored, want %d", id, passes, want)
		}
	}
	if ep := testQuizExam(mgr, tr); ep.Status != statusOK || ep.Score != 2 || ep.Max != 2 || len(ep.Questions) != 2 {
		t.Errorf("exam proof = %+v", ep)
	}
}
