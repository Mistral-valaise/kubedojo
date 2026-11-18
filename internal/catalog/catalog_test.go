package catalog_test

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Mistral-valaise/kubedojo/content"
	"github.com/Mistral-valaise/kubedojo/internal/catalog"
)

// The shipped content must always be valid.
func TestShippedContentIsValid(t *testing.T) {
	cat, err := catalog.Load(content.FS)
	if err != nil {
		t.Fatalf("content is invalid:\n%v", err)
	}
	for _, id := range []string{"cka", "ckad", "cks", "linux"} {
		tr := cat.Tracks[id]
		if tr == nil {
			t.Fatalf("track %s is missing", id)
		}
		// TEMPORARY ALLOWANCE, keyed on placeholder lessons (Track.Draft): a
		// track that is still a skeleton is held to everything except its
		// size. It ends by itself when the last placeholder lesson is replaced.
		draft := tr.Draft()
		if draft {
			t.Logf("%s is a draft (placeholder lessons): questions per domain and exam size are not checked yet", id)
		}
		perDomain := map[string]int{}
		for _, q := range tr.Questions {
			perDomain[q.Domain]++
			if len(q.Hints) == 0 {
				t.Errorf("%s/%s has no hints", id, q.ID)
			}
		}
		if len(tr.Questions) == 0 {
			t.Errorf("%s has no questions", id)
		}
		if draft {
			continue
		}
		for _, d := range tr.Domains {
			if perDomain[d.ID] == 0 {
				t.Errorf("%s: domain %s has no questions", id, d.ID)
			}
		}
		if len(tr.Questions) < tr.Exam.Questions {
			t.Errorf("%s has %d questions, fewer than one exam (%d)", id, len(tr.Questions), tr.Exam.Questions)
		}
	}
}

// A track is a draft exactly as long as one of its lessons is a placeholder.
func TestDraftTrack(t *testing.T) {
	fsys := questionFS(fmt.Sprintf(baseQuestion, "", ""), nil)
	cat, err := catalog.Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if cat.Tracks["t"].Draft() {
		t.Error("a track with a written lesson is a draft")
	}
	fsys["t/lessons/d.md"] = &fstest.MapFile{Data: []byte("# D\n\n" + catalog.PlaceholderLesson + ": the real one is being written.\n")}
	if cat, err = catalog.Load(fsys); err != nil {
		t.Fatal(err)
	}
	if !cat.Tracks["t"].Draft() {
		t.Error("a track with a placeholder lesson is not a draft")
	}
}

const trackYAML = `
title: T
kubernetesVersion: v1.37
exam: {durationMinutes: 10, passPercent: 50, questions: 1}
domains: [{id: d, title: D, weight: 100}]
environments: {base: {}}
`

func questionFS(question string, extra map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{
		"t/track.yaml":                {Data: []byte(trackYAML)},
		"t/lessons/d.md":              {Data: []byte("# D")},
		"t/questions/q/question.yaml": {Data: []byte(question)},
		"t/questions/q/solution.sh":   {Data: []byte("true")},
		"t/questions/q/solution.md":   {Data: []byte("do it")},
	}
	for name, body := range extra {
		fsys["t/questions/q/"+name] = &fstest.MapFile{Data: []byte(body)}
	}
	return fsys
}

const baseQuestion = `
title: Q
domain: d
task: do
hints: [h]
checks:
  - {id: a, title: A, type: exec, run: "true", explain: e%s}
  - {id: b, title: B, type: exec, run: "true", explain: e%s}
`

func TestCheckTimeoutRetryGuard(t *testing.T) {
	cat, err := catalog.Load(questionFS(fmt.Sprintf(baseQuestion, ", timeout: 90s, retry: 30s", ", guard: true"), nil))
	if err != nil {
		t.Fatal(err)
	}
	q := cat.Tracks["t"].Question("q")
	a, b := q.Checks[0], q.Checks[1]
	if a.TimeoutD != 90*time.Second || a.RetryD != 30*time.Second || a.Guard {
		t.Errorf("a: timeout %s retry %s guard %v", a.TimeoutD, a.RetryD, a.Guard)
	}
	if b.TimeoutD != catalog.DefaultCheckTimeout || b.RetryD != 0 || !b.Guard {
		t.Errorf("b: timeout %s retry %s guard %v", b.TimeoutD, b.RetryD, b.Guard)
	}
	for _, bad := range []string{", timeout: forever", ", timeout: 1h", ", retry: 10m", ", guard: true"} {
		// The last case makes both checks guards: nothing would fail unsolved.
		other := ""
		if bad == ", guard: true" {
			other = bad
		}
		if _, err := catalog.Load(questionFS(fmt.Sprintf(baseQuestion, bad, other), nil)); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestVariants(t *testing.T) {
	ok := map[string]string{
		"variants/other-way.sh": "# expect: pass\ntrue\n",
		"variants/too-broad.sh": "# A catch-all that logs everything.\n# expect: fail b, a\ntrue\n",
		"variants/README.md":    "ignored",
	}
	cat, err := catalog.Load(questionFS(fmt.Sprintf(baseQuestion, "", ""), ok))
	if err != nil {
		t.Fatal(err)
	}
	vs := cat.Tracks["t"].Question("q").Variants
	if len(vs) != 2 || vs[0].Name != "other-way" || !vs[0].Pass || vs[1].Pass || strings.Join(vs[1].Fails, ",") != "b,a" {
		t.Fatalf("variants = %+v", vs)
	}
	for name, body := range map[string]string{
		"no header":     "true\n",
		"unknown check": "# expect: fail nope\n",
		"fail no ids":   "# expect: fail\n",
		"pass with ids": "# expect: pass a\n",
	} {
		if _, err := catalog.Load(questionFS(fmt.Sprintf(baseQuestion, "", ""), map[string]string{"variants/x.sh": body})); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestFragileCheckWarnings(t *testing.T) {
	q := `
title: Q
domain: d
task: do
hints: [h]
checks:
  - id: window
    title: W
    type: exec
    explain: e
    run: |-
      kubectl get nodes; sleep 3; tail -n 400 /var/log/kubernetes/audit/audit.log | jq -r .requestURI | wc -l | xargs
    expect: {equals: "0"}
  - id: probe
    title: P
    type: exec
    explain: e
    run: |-
      kubectl -n x run probe-$RANDOM --rm -i --restart=Never --image=busybox:1.36 -- wget -qO- http://web | grep -q ok && echo reachable
    expect: {equals: reachable}
  - id: exec-probe
    title: E
    type: exec
    explain: e
    run: |-
      kubectl -n x exec deploy/client -- wget -qO- http://web | grep -q ok && echo reachable
    expect: {equals: reachable}
  - id: fine
    title: F
    type: exec
    explain: e
    run: |-
      kubectl -n x get deploy web -o name >/dev/null || exit 1; kubectl -n x get pods -o name | wc -l
    expect: {equals: "0"}
`
	cat, err := catalog.Load(questionFS(q, nil))
	if err != nil {
		t.Fatal(err)
	}
	w := strings.Join(cat.Tracks["t"].Question("q").Warnings, "\n")
	for _, want := range []string{"check window: reads the tail of a log file", "check window: uses a fixed sleep", `check window: expects "0"`, "check probe: starts a throwaway Pod"} {
		if !strings.Contains(w, want) {
			t.Errorf("missing warning %q in:\n%s", want, w)
		}
	}
	if strings.Count(w, "\n") != 3 {
		t.Errorf("want exactly 4 warnings, got:\n%s", w)
	}
}
