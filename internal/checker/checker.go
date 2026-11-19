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

	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox"
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
	// Debug data: how the command ended, on which machine, and how long the
	// grader waited. Stderr is clipped and has credentials redacted.
	On         string `json:"on"`
	ExitCode   int    `json:"exitCode"`
	Stderr     string `json:"stderr,omitempty"`
	DurationMs int64  `json:"durationMs"`
	Attempts   int    `json:"attempts"`
	TimedOut   bool   `json:"timedOut,omitempty"`
	Guard      bool   `json:"guard,omitempty"`
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
	return RunSettled(ctx, q, run, 0)
}

// RunOnce grades like Run, but looks only once at the checks once selects,
// whatever their retry says. The content proof uses it for checks it expects
// to fail: waiting out a retry there only delays a known answer.
func RunOnce(ctx context.Context, q *catalog.Question, run Runner, once func(*catalog.Check) bool) Result {
	res := Result{QuestionID: q.ID, Max: q.Points, At: time.Now().UTC()}
	for _, c := range q.Checks {
		if once != nil && once(c) && c.RetryD > 0 {
			single := *c
			single.RetryD = 0
			c = &single
		}
		res.Checks = append(res.Checks, runCheck(ctx, c, run))
	}
	score(q, &res)
	return res
}

// RunSettled grades all checks of q and then keeps re-running the ones that
// failed until they pass or settle has elapsed.
func RunSettled(ctx context.Context, q *catalog.Question, run Runner, settle time.Duration) Result {
	res := Result{QuestionID: q.ID, Max: q.Points, At: time.Now().UTC()}
	for _, c := range q.Checks {
		res.Checks = append(res.Checks, runCheck(ctx, c, run))
	}
	score(q, &res)
	end := time.Now().Add(settle)
	for settle > 0 && !res.Passed && time.Now().Add(retryPause).Before(end) && sleepCtx(ctx, retryPause) == nil {
		res = Recheck(ctx, q, run, res)
	}
	return res
}

// Recheck runs the failed checks of an earlier result again and keeps the
// passed ones. Exam grading uses it: a Deployment fixed seconds before the
// deadline is still rolling out, and a single look would grade the rollout
// instead of the answer. A pass can never turn into a failure this way.
func Recheck(ctx context.Context, q *catalog.Question, run Runner, prev Result) Result {
	res := Result{QuestionID: q.ID, Max: q.Points, At: time.Now().UTC(), Checks: append([]CheckResult(nil), prev.Checks...)}
	for i, c := range q.Checks {
		if i >= len(res.Checks) || res.Checks[i].Passed || ctx.Err() != nil {
			continue
		}
		old := res.Checks[i]
		cr := runCheck(ctx, c, run)
		cr.Attempts += old.Attempts
		cr.DurationMs += old.DurationMs
		res.Checks[i] = cr
	}
	score(q, &res)
	return res
}

// score adds up the points of the passed checks. Guards only count once a
// check that needs work has passed: a guard holds on the untouched
// environment ("the service still runs", "the base was not edited"), so on
// its own it says nothing was broken, not that anything was done. Without
// this an exam would pay for tasks nobody opened.
func score(q *catalog.Question, res *Result) {
	res.Score = 0
	worked, guards := false, 0
	for i, c := range q.Checks {
		if i >= len(res.Checks) || !res.Checks[i].Passed {
			continue
		}
		if c.Guard {
			guards += c.Points
		} else {
			worked = true
			res.Score += c.Points
		}
	}
	if worked {
		res.Score += guards
	}
	res.Passed = res.Score == res.Max
}

// retryPause is the wait between two attempts of a retried check.
var retryPause = 2 * time.Second

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// runCheck runs one check, repeating it while it fails when the check asks
// for that with retry.
func runCheck(ctx context.Context, c *catalog.Check, run Runner) CheckResult {
	start := time.Now()
	end := start.Add(c.RetryD)
	var cr CheckResult
	for n := 1; ; n++ {
		cr = attempt(ctx, c, run)
		cr.Attempts = n
		if cr.Passed || c.RetryD == 0 || time.Now().Add(retryPause).After(end) || sleepCtx(ctx, retryPause) != nil {
			break
		}
	}
	cr.DurationMs = time.Since(start).Milliseconds()
	return cr
}

func attempt(ctx context.Context, c *catalog.Check, run Runner) CheckResult {
	cr := CheckResult{ID: c.ID, Title: c.Title, Points: c.Points, Explain: strings.TrimSpace(c.Explain),
		Command: Command(c), Expected: describe(c), Guard: c.Guard}
	cr.On = c.On
	if cr.On == "" {
		cr.On = "control-plane"
	}
	timeout := c.TimeoutD
	if timeout <= 0 {
		timeout = catalog.DefaultCheckTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := run(cctx, cr.On, cr.Command+"\n")
	cr.ExitCode = out.ExitCode
	cr.Stderr = clip(Redact(strings.TrimSpace(out.Stderr)))
	// A command that outlived its budget is reported as such, whatever the
	// runner made of the killed process (an error or exit code -1).
	if cctx.Err() == context.DeadlineExceeded && ctx.Err() == nil {
		cr.TimedOut = true
		cr.Observed = fmt.Sprintf("the check did not finish within %s", timeout)
		return cr
	}
	if err != nil {
		cr.Observed = "grader error: " + Redact(err.Error())
		return cr
	}
	stdout := strings.TrimSpace(out.Stdout)

	// Existence semantics for resource/file checks without matchers.
	if c.Type == "resource" && c.Absent {
		cr.Passed = out.ExitCode != 0 && strings.Contains(out.Stderr, "NotFound")
		if cr.Passed {
			cr.Observed = "not found"
		} else if out.ExitCode == 0 {
			cr.Observed = "still exists"
		} else {
			cr.Observed = cr.Stderr
		}
		return cr
	}
	wantExit := 0
	if c.Expect != nil && c.Expect.ExitCode != nil {
		wantExit = *c.Expect.ExitCode
	}
	if out.ExitCode != wantExit {
		msg := cr.Stderr
		if msg == "" {
			msg = clip(Redact(stdout))
		}
		switch {
		case c.Type == "resource" && strings.Contains(msg, "NotFound"):
			cr.Observed = "resource not found"
		case c.Type == "file":
			cr.Observed = "file not found or unreadable"
		case strings.Contains(msg, "array index out of bounds"):
			// kubectl's jsonpath error for "the list is empty" is long and cryptic.
			cr.Observed = "no matching object found"
		default:
			cr.Observed = fmt.Sprintf("exit code %d: %s", out.ExitCode, msg)
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

var redactions = []*regexp.Regexp{
	// PEM blocks (keys and certificates).
	regexp.MustCompile(`(?s)-----BEGIN [A-Z ]+-----.*?(-----END [A-Z ]+-----|$)`),
	// JSON Web Tokens (ServiceAccount tokens).
	regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]*`),
	// Authorization headers.
	regexp.MustCompile(`(?i)(bearer|basic)\s+[A-Za-z0-9._~+/=-]{8,}`),
}

var keyValueRe = regexp.MustCompile(`(?i)\b([a-z0-9_.-]*(password|passwd|secret|token|api[_-]?key|client-key-data|private[_-]?key)[a-z0-9_.-]*)("?\s*[:=]\s*"?)([^\s"',;]+)`)

// Redact removes credentials from grader debug output before it is stored or
// shown: the stderr of a check may quote a kubeconfig, a token or a key.
func Redact(s string) string {
	for _, re := range redactions {
		s = re.ReplaceAllString(s, "[redacted]")
	}
	return keyValueRe.ReplaceAllString(s, "${1}${3}[redacted]")
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
		if v := strings.TrimSpace(*e.Equals); strings.Contains(v, "\n") {
			// A value of several lines is shown as it is: quoted, its line
			// breaks would read as a literal \n.
			parts = append(parts, "equals:\n"+v)
		} else {
			parts = append(parts, "equals "+strconv.Quote(v))
		}
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
