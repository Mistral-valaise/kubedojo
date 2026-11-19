package checker

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox"
)

func ptr[T any](v T) *T { return &v }

func TestMatch(t *testing.T) {
	cases := []struct {
		name string
		e    *catalog.Expect
		got  string
		want bool
	}{
		{"nil expect", nil, "anything", true},
		{"equals", &catalog.Expect{Equals: ptr("yes")}, "yes", true},
		{"equals trims expectation", &catalog.Expect{Equals: ptr("yes\n")}, "yes", true},
		{"equals mismatch", &catalog.Expect{Equals: ptr("yes")}, "no", false},
		{"notEquals", &catalog.Expect{NotEquals: ptr("true")}, "", true},
		{"contains all", &catalog.Expect{Contains: []string{"a", "b"}}, "xaxbx", true},
		{"contains missing", &catalog.Expect{Contains: []string{"a", "z"}}, "xaxbx", false},
		{"notContains", &catalog.Expect{NotContains: []string{"secret"}}, "public", true},
		{"matches", &catalog.Expect{Matches: `^nginx:1\.27`}, "nginx:1.27-alpine", true},
		{"matches fails", &catalog.Expect{Matches: `^nginx:1\.27`}, "httpd:2.4", false},
		{"oneOf", &catalog.Expect{OneOf: []string{"a", "b"}}, "b", true},
		{"sameLines any order", &catalog.Expect{SameLines: []string{"b", "a"}}, "a\n b \n", true},
		{"sameLines extra", &catalog.Expect{SameLines: []string{"a"}}, "a\nb", false},
		{"sameItems", &catalog.Expect{SameItems: []string{"x", "y"}}, "y  x", true},
		{"gte", &catalog.Expect{Gte: ptr(2.0)}, "3", true},
		{"gte below", &catalog.Expect{Gte: ptr(2.0)}, "1", false},
		{"gte not a number", &catalog.Expect{Gte: ptr(2.0)}, "abc", false},
		{"notEmpty", &catalog.Expect{NotEmpty: true}, "", false},
	}
	for _, c := range cases {
		if got, _ := match(c.e, c.got); got != c.want {
			t.Errorf("%s: match(%q) = %v, want %v", c.name, c.got, got, c.want)
		}
	}
}

func TestRunScoresChecks(t *testing.T) {
	q := &catalog.Question{ID: "q", Points: 5, Checks: []*catalog.Check{
		{ID: "exists", Title: "exists", Points: 1, Type: "resource", Resource: "pod/a", Namespace: "ns"},
		{ID: "gone", Title: "gone", Points: 1, Type: "resource", Resource: "pod/b", Absent: true},
		{ID: "deny", Title: "deny", Points: 2, Type: "exec", Run: "can-i", Expect: &catalog.Expect{Equals: ptr("no"), ExitCode: ptr(1)}},
		{ID: "file", Title: "file", Points: 1, Type: "file", Path: "/x", Expect: &catalog.Expect{Equals: ptr("42")}},
	}}
	run := func(_ context.Context, role, script string) (sandbox.ExecResult, error) {
		switch {
		case script == "kubectl get pod/a -n ns -o name\n":
			return sandbox.ExecResult{Stdout: "pod/a\n"}, nil
		case script == "kubectl get pod/b -o name\n":
			return sandbox.ExecResult{ExitCode: 1, Stderr: `Error from server (NotFound): pods "b" not found`}, nil
		case script == "can-i\n":
			return sandbox.ExecResult{Stdout: "no\n", ExitCode: 1}, nil
		default: // cat '/x'
			return sandbox.ExecResult{ExitCode: 1, Stderr: "No such file"}, nil
		}
	}
	res := Run(context.Background(), q, run)
	if res.Score != 4 || res.Max != 5 || res.Passed {
		t.Fatalf("score %d/%d passed=%v, want 4/5 false", res.Score, res.Max, res.Passed)
	}
	if res.Checks[3].Observed != "file not found or unreadable" {
		t.Errorf("file check observed %q", res.Checks[3].Observed)
	}
}

func chk(id string, e *catalog.Expect) *catalog.Check {
	return &catalog.Check{ID: id, Title: id, Points: 1, Type: "exec", Run: id, Expect: e}
}

// A pipeline that ends in "wc -l" exits 0 and prints 0 even when the command
// in front of it failed. The engine can only see exit 0 and "0": such checks
// must assert the precondition themselves and exit non-zero. This pins the
// engine side: a non-zero exit never passes, whatever was printed.
func TestFailedCommandNeverPasses(t *testing.T) {
	q := &catalog.Question{ID: "q", Points: 3, Checks: []*catalog.Check{
		chk("zero", &catalog.Expect{Equals: ptr("0")}),
		chk("empty", &catalog.Expect{NotEquals: ptr("true")}),
		chk("none", nil),
	}}
	run := func(context.Context, string, string) (sandbox.ExecResult, error) {
		return sandbox.ExecResult{Stdout: "0\n", ExitCode: 1, Stderr: "audit log missing"}, nil
	}
	res := Run(context.Background(), q, run)
	if res.Score != 0 {
		t.Fatalf("score %d, want 0: a failed command must not pass", res.Score)
	}
	c := res.Checks[0]
	if c.ExitCode != 1 || c.Stderr != "audit log missing" || c.Attempts != 1 || c.On != "control-plane" {
		t.Errorf("debug data = exit %d stderr %q attempts %d on %q", c.ExitCode, c.Stderr, c.Attempts, c.On)
	}
	if c.Observed != "exit code 1: audit log missing" {
		t.Errorf("observed %q", c.Observed)
	}
}

func TestTimeoutIsReported(t *testing.T) {
	c := chk("slow", nil)
	c.TimeoutD = 20 * time.Millisecond
	q := &catalog.Question{ID: "q", Points: 1, Checks: []*catalog.Check{c}}
	run := func(ctx context.Context, _, _ string) (sandbox.ExecResult, error) {
		<-ctx.Done()
		// What the kind driver returns when the docker CLI is killed.
		return sandbox.ExecResult{ExitCode: -1}, nil
	}
	res := Run(context.Background(), q, run)
	got := res.Checks[0]
	if got.Passed || !got.TimedOut || !strings.Contains(got.Observed, "did not finish within 20ms") {
		t.Fatalf("passed=%v timedOut=%v observed=%q", got.Passed, got.TimedOut, got.Observed)
	}
}

func TestRetryUntilPass(t *testing.T) {
	defer func(d time.Duration) { retryPause = d }(retryPause)
	retryPause = time.Millisecond
	c := chk("converges", &catalog.Expect{Equals: ptr("ready")})
	c.RetryD = 2 * time.Second
	never := chk("never", &catalog.Expect{Equals: ptr("ready")})
	never.RetryD = 30 * time.Millisecond
	q := &catalog.Question{ID: "q", Points: 2, Checks: []*catalog.Check{c, never}}
	calls := 0
	run := func(_ context.Context, _, script string) (sandbox.ExecResult, error) {
		if script == "never\n" {
			return sandbox.ExecResult{Stdout: "pending"}, nil
		}
		calls++
		if calls < 3 {
			return sandbox.ExecResult{Stdout: "pending"}, nil
		}
		return sandbox.ExecResult{Stdout: "ready"}, nil
	}
	res := Run(context.Background(), q, run)
	if !res.Checks[0].Passed || res.Checks[0].Attempts != 3 {
		t.Errorf("converging check: passed=%v attempts=%d, want true/3", res.Checks[0].Passed, res.Checks[0].Attempts)
	}
	if res.Checks[1].Passed || res.Checks[1].Attempts < 2 {
		t.Errorf("check that never converges: passed=%v attempts=%d", res.Checks[1].Passed, res.Checks[1].Attempts)
	}
}

// The content proof looks once at checks it expects to fail: their retry must
// not be waited out, while the other checks keep theirs.
func TestRunOnceSkipsRetryOfSelectedChecks(t *testing.T) {
	defer func(d time.Duration) { retryPause = d }(retryPause)
	retryPause = time.Millisecond
	expected := chk("expected-fail", &catalog.Expect{Equals: ptr("ready")})
	expected.RetryD = time.Hour
	converges := chk("converges", &catalog.Expect{Equals: ptr("ready")})
	converges.RetryD = 2 * time.Second
	q := &catalog.Question{ID: "q", Points: 2, Checks: []*catalog.Check{expected, converges}}
	calls := 0
	run := func(_ context.Context, _, script string) (sandbox.ExecResult, error) {
		if script == "expected-fail\n" {
			return sandbox.ExecResult{Stdout: "pending"}, nil
		}
		calls++
		if calls < 3 {
			return sandbox.ExecResult{Stdout: "pending"}, nil
		}
		return sandbox.ExecResult{Stdout: "ready"}, nil
	}
	start := time.Now()
	res := RunOnce(context.Background(), q, run, func(c *catalog.Check) bool { return c.ID == "expected-fail" })
	if res.Checks[0].Passed || res.Checks[0].Attempts != 1 {
		t.Errorf("selected check: passed=%v attempts=%d, want false/1", res.Checks[0].Passed, res.Checks[0].Attempts)
	}
	if !res.Checks[1].Passed || res.Checks[1].Attempts != 3 {
		t.Errorf("other check: passed=%v attempts=%d, want true/3", res.Checks[1].Passed, res.Checks[1].Attempts)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("took %v: the selected check's retry was waited out", d)
	}
	if expected.RetryD != time.Hour {
		t.Error("RunOnce changed the check of the catalog")
	}
}

// Exam grading re-runs failed checks only: a pass is final, and state that
// converges within the window is graded once it has settled.
func TestRecheckKeepsPasses(t *testing.T) {
	defer func(d time.Duration) { retryPause = d }(retryPause)
	retryPause = time.Millisecond
	q := &catalog.Question{ID: "q", Points: 3, Checks: []*catalog.Check{
		chk("stable", &catalog.Expect{Equals: ptr("ok")}),
		chk("late", &catalog.Expect{Equals: ptr("ok")}),
		chk("wrong", &catalog.Expect{Equals: ptr("ok")}),
	}}
	n := map[string]int{}
	run := func(_ context.Context, _, script string) (sandbox.ExecResult, error) {
		id := strings.TrimSpace(script)
		n[id]++
		switch {
		case id == "stable" && n[id] == 1, id == "late" && n[id] >= 3:
			return sandbox.ExecResult{Stdout: "ok"}, nil
		}
		return sandbox.ExecResult{Stdout: "no"}, nil
	}
	res := RunSettled(context.Background(), q, run, 200*time.Millisecond)
	if n["stable"] != 1 {
		t.Errorf("a passed check ran %d times, want 1", n["stable"])
	}
	if res.Score != 2 || res.Passed {
		t.Fatalf("score %d passed=%v, want 2 false", res.Score, res.Passed)
	}
	if !res.Checks[1].Passed || res.Checks[1].Attempts != 3 {
		t.Errorf("late check: passed=%v attempts=%d", res.Checks[1].Passed, res.Checks[1].Attempts)
	}
	if res.Checks[2].Passed {
		t.Error("a wrong answer passed after settling")
	}
}

func TestRedact(t *testing.T) {
	jwt := "eyJhbGciOiJSUzI1NiIsImtpZCI6IngifQ.eyJpc3MiOiJrdWJlcm5ldGVzIn0.c2lnbmF0dXJl"
	in := "token: " + jwt + "\npassword=hunter2 user=admin\nAuthorization: Bearer abcdef123456\n" +
		"-----BEGIN PRIVATE KEY-----\nMIIEvQ\n-----END PRIVATE KEY-----\nclient-key-data: LS0tLS1CRUdJTg==\nError from server (NotFound): pods \"x\" not found"
	out := Redact(in)
	for _, leak := range []string{jwt, "hunter2", "abcdef123456", "MIIEvQ", "LS0tLS1CRUdJTg=="} {
		if strings.Contains(out, leak) {
			t.Errorf("redacted output still contains %q:\n%s", leak, out)
		}
	}
	for _, keep := range []string{"user=admin", `pods "x" not found`} {
		if !strings.Contains(out, keep) {
			t.Errorf("redaction removed harmless text %q:\n%s", keep, out)
		}
	}
}

// A check describes what it expects for the learner. A value of several lines
// keeps its line breaks; quoted, they would read as a literal \n.
func TestDescribeKeepsLineBreaks(t *testing.T) {
	if got := describe(chk("c", &catalog.Expect{Equals: ptr("one\n")})); got != `equals "one"` {
		t.Errorf("one line: %q", got)
	}
	if got := describe(chk("c", &catalog.Expect{Equals: ptr("one\ntwo\n")})); got != "equals:\none\ntwo" {
		t.Errorf("two lines: %q", got)
	}
}
