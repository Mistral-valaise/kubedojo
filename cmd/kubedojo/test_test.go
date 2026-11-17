package main

import (
	"strings"
	"testing"

	"github.com/Mistral-valaise/kubedojo/internal/checker"
)

func result(passed map[string]bool, guards ...string) *checker.Result {
	g := map[string]bool{}
	for _, id := range guards {
		g[id] = true
	}
	res := &checker.Result{}
	for _, id := range []string{"a", "b", "c"} {
		res.Checks = append(res.Checks, checker.CheckResult{ID: id, Passed: passed[id], Guard: g[id], Expected: "x", Observed: "y"})
	}
	return res
}

func TestUnsolvedPasses(t *testing.T) {
	if got := unsolvedPasses(result(map[string]bool{"a": true, "c": true}, "c")); len(got) != 1 || got[0] != "a" {
		t.Errorf("unsolvedPasses = %v, want [a]: a guard may pass unsolved, any other check may not", got)
	}
	if got := unsolvedPasses(result(nil)); len(got) != 0 {
		t.Errorf("unsolvedPasses = %v, want none", got)
	}
}

func TestMismatch(t *testing.T) {
	all := map[string]bool{"a": true, "b": true, "c": true}
	if bad := mismatch(result(all), nil); len(bad) != 0 {
		t.Errorf("a full pass does not match a correct solution: %v", bad)
	}
	// A wrong variant that is expected to fail b, but everything passes: the
	// grader let a wrong answer through.
	bad := mismatch(result(all), []string{"b"})
	if len(bad) != 1 || !bad[0].wantFail || !strings.Contains(describeMismatch(bad), "[b] passes but must fail") {
		t.Errorf("mismatch = %+v", bad)
	}
	// The declared outcome: only b fails.
	if bad := mismatch(result(map[string]bool{"a": true, "c": true}), []string{"b"}); len(bad) != 0 {
		t.Errorf("declared outcome reported as mismatch: %v", bad)
	}
	// It fails more than it declared.
	bad = mismatch(result(map[string]bool{"a": true}), []string{"b"})
	if len(bad) != 1 || bad[0].wantFail || bad[0].check.ID != "c" {
		t.Errorf("mismatch = %+v", bad)
	}
}
