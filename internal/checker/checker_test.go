package checker

import (
	"context"
	"testing"

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
