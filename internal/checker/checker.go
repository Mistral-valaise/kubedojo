// Package checker grades a question by running its checks inside the sandbox.
package checker

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kubedojo/kubedojo/internal/catalog"
	"github.com/kubedojo/kubedojo/internal/sandbox"
)

// Runner executes a script on a sandbox machine identified by role.
type Runner func(ctx context.Context, role, script string) (sandbox.ExecResult, error)

type CheckResult struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Points   int    `json:"points"`
	Passed   bool   `json:"passed"`
	Expected string `json:"expected"`
	Observed string `json:"observed"`
	Explain  string `json:"explain"`
	Command  string `json:"command"`
}

type Result struct {
	QuestionID string        `json:"questionId"`
	Score      int           `json:"score"`
	Max        int           `json:"max"`
	Passed     bool          `json:"passed"`
	Checks     []CheckResult `json:"checks"`
	At         time.Time     `json:"at"`
}

func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// Command renders the shell command a check runs.
func Command(c *catalog.Check) string {
	switch c.Type {
	case "resource":
		cmd := "kubectl get " + c.Resource
		if c.Namespace != "" {
			cmd += " -n " + c.Namespace
		}
		if c.JSONPath != "" {
			cmd += " -o jsonpath=" + shQuote(c.JSONPath)
		} else {
			cmd += " -o name"
		}
		return cmd
	case "file":
		return "cat " + shQuote(c.Path)
	default:
		return strings.TrimSpace(c.Run)
	}
}

// Run grades all checks of q.
func Run(ctx context.Context, q *catalog.Question, run Runner) Result {
	res := Result{QuestionID: q.ID, Max: q.Points, At: time.Now().UTC()}
	for _, c := range q.Checks {
		cr := runCheck(ctx, c, run)
		if cr.Passed {
			res.Score += c.Points
		}
		res.Checks = append(res.Checks, cr)
	}
	res.Passed = res.Score == res.Max
	return res
}

func runCheck(ctx context.Context, c *catalog.Check, run Runner) CheckResult {
	cr := CheckResult{ID: c.ID, Title: c.Title, Points: c.Points, Explain: strings.TrimSpace(c.Explain), Command: Command(c)}
	role := c.On
	if role == "" {
		role = "control-plane"
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out, err := run(cctx, role, cr.Command+"\n")
	if err != nil {
		cr.Expected = describe(c)
		cr.Observed = "grader error: " + err.Error()
		return cr
	}
	stdout := strings.TrimSpace(out.Stdout)
	cr.Expected = describe(c)

	// Existence semantics for resource/file checks without matchers.
	if c.Type == "resource" && c.Absent {
		cr.Passed = out.ExitCode != 0 && strings.Contains(out.Stderr, "NotFound")
		if cr.Passed {
			cr.Observed = "not found"
		} else if out.ExitCode == 0 {
			cr.Observed = "still exists"
		} else {
			cr.Observed = clip(strings.TrimSpace(out.Stderr))
		}
		return cr
	}
	wantExit := 0
	if c.Expect != nil && c.Expect.ExitCode != nil {
		wantExit = *c.Expect.ExitCode
	}
	if out.ExitCode != wantExit {
		msg := strings.TrimSpace(out.Stderr)
		if msg == "" {
			msg = stdout
		}
		switch {
		case c.Type == "resource" && strings.Contains(msg, "NotFound"):
			cr.Observed = "resource not found"
		case c.Type == "file":
			cr.Observed = "file not found or unreadable"
		default:
			cr.Observed = fmt.Sprintf("exit code %d: %s", out.ExitCode, clip(msg))
		}
		return cr
	}
	ok, why := match(c.Expect, stdout)
	cr.Passed = ok
	if stdout == "" {
		cr.Observed = "(empty)"
	} else {
		cr.Observed = clip(stdout)
	}
	if !ok && why != "" {
		cr.Observed += "\n→ " + why
	}
	return cr
}

func clip(s string) string {
	const max = 600
	if len(s) > max {
		return s[:max] + " …"
	}
	return s
}

func normLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	sort.Strings(out)
	return out
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func match(e *catalog.Expect, got string) (bool, string) {
	if e == nil {
		return true, ""
	}
	if e.Equals != nil && got != strings.TrimSpace(*e.Equals) {
		return false, "value differs from the expected one"
	}
	if e.NotEquals != nil && got == strings.TrimSpace(*e.NotEquals) {
		return false, "value must not be " + strconv.Quote(*e.NotEquals)
	}
	for _, s := range e.Contains {
		if !strings.Contains(got, s) {
			return false, "missing " + strconv.Quote(s)
		}
	}
	for _, s := range e.NotContains {
		if strings.Contains(got, s) {
			return false, "must not contain " + strconv.Quote(s)
		}
	}
	if e.Matches != "" {
		if ok, _ := regexp.MatchString(e.Matches, got); !ok {
			return false, "does not match pattern " + e.Matches
		}
	}
	if len(e.OneOf) > 0 {
		found := false
		for _, s := range e.OneOf {
			if got == s {
				found = true
			}
		}
		if !found {
			return false, "not one of the accepted values"
		}
	}
	if len(e.SameLines) > 0 {
		want := normLines(strings.Join(e.SameLines, "\n"))
		if !sameSet(want, normLines(got)) {
			return false, "lines differ from the expected set"
		}
	}
	if len(e.SameItems) > 0 {
		want := append([]string(nil), e.SameItems...)
		sort.Strings(want)
		have := strings.Fields(got)
		sort.Strings(have)
		if !sameSet(want, have) {
			return false, "items differ from the expected set"
		}
	}
	if e.Gte != nil || e.Lte != nil {
		n, err := strconv.ParseFloat(strings.TrimSpace(got), 64)
		if err != nil {
			return false, "not a number"
		}
		if e.Gte != nil && n < *e.Gte {
			return false, fmt.Sprintf("must be ≥ %v", *e.Gte)
		}
		if e.Lte != nil && n > *e.Lte {
			return false, fmt.Sprintf("must be ≤ %v", *e.Lte)
		}
	}
	if e.NotEmpty && got == "" {
		return false, "must not be empty"
	}
	return true, ""
}

func describe(c *catalog.Check) string {
	if c.Type == "resource" && c.Absent {
		return "resource must not exist"
	}
	e := c.Expect
	if e == nil {
		switch c.Type {
		case "resource":
			return "resource exists"
		case "file":
			return "file exists"
		default:
			return "command succeeds (exit code 0)"
		}
	}
	var parts []string
	if e.Equals != nil {
		parts = append(parts, "equals "+strconv.Quote(strings.TrimSpace(*e.Equals)))
	}
	if e.NotEquals != nil {
		parts = append(parts, "not "+strconv.Quote(*e.NotEquals))
	}
	if len(e.Contains) > 0 {
		parts = append(parts, "contains "+strings.Join(quoteAll(e.Contains), ", "))
	}
	if len(e.NotContains) > 0 {
		parts = append(parts, "does not contain "+strings.Join(quoteAll(e.NotContains), ", "))
	}
	if e.Matches != "" {
		parts = append(parts, "matches /"+e.Matches+"/")
	}
	if len(e.OneOf) > 0 {
		parts = append(parts, "one of "+strings.Join(quoteAll(e.OneOf), ", "))
	}
	if len(e.SameLines) > 0 {
		parts = append(parts, "exactly these lines (any order): "+strings.Join(e.SameLines, " | "))
	}
	if len(e.SameItems) > 0 {
		parts = append(parts, "exactly these items (any order): "+strings.Join(e.SameItems, " "))
	}
	if e.Gte != nil {
		parts = append(parts, fmt.Sprintf("≥ %v", *e.Gte))
	}
	if e.Lte != nil {
		parts = append(parts, fmt.Sprintf("≤ %v", *e.Lte))
	}
	if e.NotEmpty {
		parts = append(parts, "not empty")
	}
	if e.ExitCode != nil {
		parts = append(parts, fmt.Sprintf("exit code %d", *e.ExitCode))
	}
	return strings.Join(parts, "; ")
}

func quoteAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strconv.Quote(s)
	}
	return out
}
