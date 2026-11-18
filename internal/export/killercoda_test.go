package export

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Mistral-valaise/kubedojo/content"
	"github.com/Mistral-valaise/kubedojo/internal/catalog"
)

func ptr[T any](v T) *T { return &v }

// The generated verify.sh must agree with the Go matchers: it exits 0 when
// every check holds and 1 as soon as one does not.
func TestVerifyScriptMatchers(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	mk := func(run string, e *catalog.Expect) *catalog.Question {
		return &catalog.Question{ID: "q", Checks: []*catalog.Check{{ID: "c", Title: "it's a check", Type: "exec", Run: run, Expect: e}}}
	}
	cases := []struct {
		name string
		q    *catalog.Question
		pass bool
	}{
		{"exit 0", mk("true", nil), true},
		{"exit 1", mk("false", nil), false},
		{"equals", mk("echo yes", &catalog.Expect{Equals: ptr("yes")}), true},
		{"equals mismatch", mk("echo no", &catalog.Expect{Equals: ptr("yes")}), false},
		{"expected exit code", mk("echo no; exit 1", &catalog.Expect{Equals: ptr("no"), ExitCode: ptr(1)}), true},
		{"multi-line equals", mk(`printf 'a\nb\n'`, &catalog.Expect{Equals: ptr("a\nb")}), true},
		{"contains", mk("echo 'CN=dana,O=auditors'", &catalog.Expect{Contains: []string{"CN=dana"}}), true},
		{"contains missing", mk("echo other", &catalog.Expect{Contains: []string{"CN=dana"}}), false},
		{"notContains", mk("echo public", &catalog.Expect{NotContains: []string{"secret"}}), true},
		{"notContains hit", mk("echo secret", &catalog.Expect{NotContains: []string{"secret"}}), false},
		{"matches with \\d", mk("echo 10.96.0.1", &catalog.Expect{Matches: `^\d+\.\d+\.\d+\.\d+$`}), true},
		{"matches fails", mk("echo nope", &catalog.Expect{Matches: `^\d+$`}), false},
		{"matches with \\S", mk("echo 'Running kd-control-plane'", &catalog.Expect{Matches: `^Running \S+-control-plane$`}), true},
		{"sameLines any order", mk(`printf 'b\na\n'`, &catalog.Expect{SameLines: []string{"a", "b"}}), true},
		{"sameLines extra", mk(`printf 'a\nb\nc\n'`, &catalog.Expect{SameLines: []string{"a", "b"}}), false},
		{"gte", mk("echo 4", &catalog.Expect{Gte: ptr(4.0)}), true},
		{"gte below", mk("echo 3", &catalog.Expect{Gte: ptr(4.0)}), false},
		{"notEquals", mk("echo false", &catalog.Expect{NotEquals: ptr("true")}), true},
		{"notEquals hit", mk("echo true", &catalog.Expect{NotEquals: ptr("true")}), false},
	}
	dir := t.TempDir()
	for _, c := range cases {
		path := filepath.Join(dir, "verify.sh")
		if err := os.WriteFile(path, []byte(Verify(c.q)), 0o755); err != nil {
			t.Fatal(err)
		}
		err := exec.Command("bash", path).Run()
		if (err == nil) != c.pass {
			t.Errorf("%s: script passed=%v, want %v", c.name, err == nil, c.pass)
		}
	}
}

// A check with retry is run again until it passes; one without is not.
func TestVerifyScriptRetry(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	dir := t.TempDir()
	// Passes on the second run: the first one leaves a marker behind.
	run := "f=" + filepath.Join(dir, "seen") + "; [ -e $f ] && echo ready || { touch $f; echo pending; }"
	for _, c := range []struct {
		name  string
		retry time.Duration
		pass  bool
	}{{"without retry", 0, false}, {"with retry", 10 * time.Second, true}} {
		os.Remove(filepath.Join(dir, "seen"))
		q := &catalog.Question{ID: "q", Checks: []*catalog.Check{{ID: "c", Title: "converges", Type: "exec", Run: run,
			Expect: &catalog.Expect{Equals: ptr("ready")}, RetryD: c.retry}}}
		path := filepath.Join(dir, "verify.sh")
		if err := os.WriteFile(path, []byte(Verify(q)), 0o755); err != nil {
			t.Fatal(err)
		}
		err := exec.Command("bash", path).Run()
		if (err == nil) != c.pass {
			t.Errorf("%s: script passed=%v, want %v", c.name, err == nil, c.pass)
		}
	}
}

// Every shipped question either exports to syntactically valid scripts or is
// skipped with a reason.
func TestExportShippedContent(t *testing.T) {
	cat, err := catalog.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	res, err := Killercoda(cat, dir, "", "https://example.invalid/kubedojo")
	if err != nil {
		t.Fatal(err)
	}
	total, tasks := 0, 0
	choice := map[string]bool{}
	for _, tr := range cat.List() {
		total += len(tr.Questions)
		for _, q := range tr.Questions {
			if q.Type == catalog.TypeChoice {
				choice[tr.ID+"/"+q.ID] = true
			} else {
				tasks++
			}
		}
	}
	if len(res.Exported)+len(res.Skipped) != total {
		t.Fatalf("exported %d + skipped %d != %d questions", len(res.Exported), len(res.Skipped), total)
	}
	// Choice questions never export, so they do not count towards the share.
	if len(res.Exported) < tasks/2 {
		t.Errorf("only %d of %d tasks export", len(res.Exported), tasks)
	}
	for _, s := range res.Skipped {
		if s.Reason == "" {
			t.Errorf("%s/%s skipped without a reason", s.Track, s.Question)
		}
		delete(choice, s.Track+"/"+s.Question)
	}
	if len(choice) > 0 {
		t.Errorf("choice questions were exported as scenarios: %v", choice)
	}
	if _, err := exec.LookPath("bash"); err != nil {
		return
	}
	for _, id := range res.Exported {
		for _, f := range []string{"verify.sh", "setup.sh", "wait.sh"} {
			if out, err := exec.Command("bash", "-n", filepath.Join(dir, id, f)).CombinedOutput(); err != nil {
				t.Errorf("%s/%s: %s", id, f, out)
			}
		}
	}
}
