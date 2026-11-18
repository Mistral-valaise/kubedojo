package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/checker"
	"github.com/Mistral-valaise/kubedojo/internal/session"
	"github.com/Mistral-valaise/kubedojo/internal/store"
)

// Outcomes of proving one question. Everything except statusOK and
// statusSkipped fails the run.
const (
	statusOK            = "ok"
	statusSkipped       = "skipped"        // not runnable here; the reason says why
	statusSetupError    = "setup-error"    // the sandbox or setup.sh failed
	statusSolutionError = "solution-error" // solution.sh (or a variant) exited non-zero
	statusFalsePositive = "false-positive" // a check passes although the task is not solved
	statusFalseNegative = "false-negative" // a check fails although the task is solved
	statusFlaky         = "flaky"          // a check passed and then failed without any change
)

// testReport is the machine-readable result of `content test --report`.
type testReport struct {
	GeneratedAt time.Time        `json:"generatedAt"`
	Environment testEnvironment  `json:"environment"`
	Summary     map[string]int   `json:"summary"` // status → number of questions
	Questions   []*questionProof `json:"questions"`
	Exams       []*examProof     `json:"exams,omitempty"`
}

type testEnvironment struct {
	Kubedojo          string `json:"kubedojo"`
	KubernetesVersion string `json:"kubernetesVersion"`
	NodeImage         string `json:"nodeImage"`
	TerminalImage     string `json:"terminalImage"`
	LinuxImage        string `json:"linuxImage,omitempty"`
	Driver            string `json:"driver"`
	OS                string `json:"os"`
	Arch              string `json:"arch"`
	Repeat            int    `json:"repeat"`
	Quick             bool   `json:"quick,omitempty"`
	SettleSeconds     int    `json:"settleSeconds"`
}

type questionProof struct {
	ID          string      `json:"id"`
	Track       string      `json:"track"`
	Domain      string      `json:"domain"`
	Difficulty  string      `json:"difficulty"`
	Environment string      `json:"environment"`
	Disruptive  bool        `json:"disruptive,omitempty"`
	Fresh       bool        `json:"freshEnvironment,omitempty"`
	Checks      int         `json:"checks"`
	Points      int         `json:"points"`
	Status      string      `json:"status"`
	Reason      string      `json:"reason,omitempty"`
	Seconds     float64     `json:"seconds"`
	Runs        []*proofRun `json:"runs,omitempty"`
}

// proofRun is one pass over a question: reset, grade the untouched state,
// apply a script (solution.sh or a variant), grade until settled, grade again.
type proofRun struct {
	Run            int             `json:"run"`
	Script         string          `json:"script"` // solution.sh or variants/<name>.sh
	Expect         string          `json:"expect"` // pass | fail <ids>
	Status         string          `json:"status"`
	Reason         string          `json:"reason,omitempty"`
	Before         *checker.Result `json:"before,omitempty"`
	After          *checker.Result `json:"after,omitempty"`
	SettledSeconds float64         `json:"settledSeconds,omitempty"`
	Rechecks       int             `json:"rechecks,omitempty"`
}

type examProof struct {
	Track     string            `json:"track"`
	Status    string            `json:"status"`
	Reason    string            `json:"reason,omitempty"`
	Score     int               `json:"score"`
	Max       int               `json:"max"`
	Seconds   float64           `json:"seconds"`
	Questions []*checker.Result `json:"questions,omitempty"`
}

type proofOptions struct {
	settle   time.Duration
	repeat   int
	variants bool
	quick    bool
}

// cmdTest proves each question end to end against a real sandbox:
//  1. setup.sh runs,
//  2. every check that is not a guard FAILS on the untouched environment,
//  3. solution.sh runs,
//  4. every check passes and keeps passing when graded again,
//  5. with --repeat, the question is reset and all of it is done again,
//  6. each variant under variants/ produces exactly the grade it declares.
func cmdTest(ctx context.Context, args []string) error {
	fl := flag.NewFlagSet("content test", flag.ContinueOnError)
	var c common
	c.register(fl)
	track := fl.String("track", "", "only this track")
	only := fl.String("question", "", "only these question ids (comma separated)")
	keep := fl.Bool("keep", false, "keep the sandbox afterwards for debugging")
	settle := fl.Duration("settle", 150*time.Second, "how long to wait for checks to pass after the solution ran")
	verbose := fl.Bool("v", false, "print provisioning progress")
	exam := fl.Bool("exam", false, "run all questions of a track together as one exam (detects conflicts between questions)")
	repeat := fl.Int("repeat", 1, "prove every question this many times, resetting it in between (detects flaky checks and leftovers)")
	noVariants := fl.Bool("no-variants", false, "skip the scripts under variants/")
	quick := fl.Bool("quick", false, "look only once at checks that are expected to fail instead of waiting out their retry (faster; misses a setup that solves itself after a while)")
	reportPath := fl.String("report", "", "write a machine-readable JSON report to this file")
	if err := fl.Parse(args); err != nil {
		return err
	}
	if *repeat < 1 {
		*repeat = 1
	}
	cat, err := catalog.Load(c.contentFS())
	if err != nil {
		return fmt.Errorf("content is invalid:\n%w", err)
	}
	drv := c.driver()
	var selected []*catalog.Track
	for _, t := range cat.List() {
		if *track == "" || t.ID == *track {
			selected = append(selected, t)
		}
	}
	if err := preflight(ctx, drv, true, needsOf(selected)); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "kubedojo-test-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	st, err := store.Open(tmp + "/test.db")
	if err != nil {
		return err
	}
	defer st.Close()
	level := slog.LevelWarn
	if *verbose {
		level = slog.LevelInfo
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	mgr, err := session.NewManager(session.Config{Catalog: cat, Store: st, Driver: drv, Logger: log, MaxSessions: 10,
		GradeSettle: *settle})
	if err != nil {
		return err
	}
	defer mgr.Close()

	rep := &testReport{GeneratedAt: time.Now().UTC(), Summary: map[string]int{}, Environment: testEnvironment{
		Kubedojo: version, KubernetesVersion: c.k8sVersion, NodeImage: c.nodeImage, TerminalImage: c.termImage, LinuxImage: c.linuxImage,
		Driver: drv.Name(), OS: runtime.GOOS, Arch: runtime.GOARCH, Repeat: *repeat, Quick: *quick, SettleSeconds: int(settle.Seconds())}}
	writeReport := func() error {
		if *reportPath == "" {
			return nil
		}
		b, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(*reportPath, append(b, '\n'), 0o644)
	}

	want := map[string]bool{}
	for _, id := range strings.Split(*only, ",") {
		if id = strings.TrimSpace(id); id != "" {
			want[id] = true
		}
	}
	if *exam {
		bad := 0
		for _, t := range cat.List() {
			if *track != "" && t.ID != *track {
				continue
			}
			ep := testExam(ctx, mgr, t, *keep)
			rep.Exams = append(rep.Exams, ep)
			if ep.Status != statusOK {
				fmt.Printf("  ✗ %s: %s\n", t.ID, ep.Reason)
				bad++
			}
		}
		if err := writeReport(); err != nil {
			return err
		}
		if bad > 0 {
			return fmt.Errorf("%d exam(s) failed", bad)
		}
		return nil
	}
	opts := proofOptions{settle: *settle, repeat: *repeat, variants: !*noVariants, quick: *quick}
	failed, total := 0, 0
	for _, t := range cat.List() {
		if *track != "" && t.ID != *track {
			continue
		}
		var qs []*catalog.Question
		for _, q := range t.Questions {
			if len(want) > 0 && !want[q.ID] {
				continue
			}
			if q.Driver != drv.Name() {
				// Never silently: a question that cannot run here is listed with the reason.
				qp := newProof(q)
				qp.Status, qp.Reason = statusSkipped, fmt.Sprintf("needs driver %q, this run uses %q", q.Driver, drv.Name())
				rep.Questions = append(rep.Questions, qp)
				rep.Summary[qp.Status]++
				continue
			}
			qs = append(qs, q)
		}
		if len(qs) == 0 {
			continue
		}
		ses, err := mgr.Practice(t.ID, "")
		if err != nil {
			return err
		}
		fmt.Printf("\n%s (%d questions, session %s)\n", t.Title, len(qs), ses.ID)
		for _, q := range qs {
			total++
			qp := proveQuestion(ctx, mgr, ses.ID, q, opts)
			rep.Questions = append(rep.Questions, qp)
			rep.Summary[qp.Status]++
			if qp.Status != statusOK {
				failed++
				fmt.Printf("  ✗ %-44s %5.0fs  %s: %s\n", q.ID, qp.Seconds, qp.Status, qp.Reason)
			} else {
				fmt.Printf("  ✓ %-44s %5.0fs\n", q.ID, qp.Seconds)
			}
			_ = writeReport() // keep the report current; a long run may be interrupted
			if ctx.Err() != nil {
				break
			}
		}
		if !*keep {
			dctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			_ = mgr.Destroy(dctx, ses.ID)
			cancel()
		} else {
			fmt.Printf("  sandbox kept: docker exec -it -u candidate kd-%s-terminal bash\n", ses.ID)
		}
	}
	if n := rep.Summary[statusSkipped]; n > 0 {
		fmt.Printf("\n%d question(s) skipped:\n", n)
		for _, qp := range rep.Questions {
			if qp.Status == statusSkipped {
				fmt.Printf("  - %-44s %s\n", qp.ID, qp.Reason)
			}
		}
	}
	fmt.Printf("\n%d of %d questions passed\n", total-failed, total)
	if err := writeReport(); err != nil {
		return err
	}
	if *reportPath != "" {
		fmt.Printf("report written to %s\n", *reportPath)
	}
	if failed > 0 {
		return fmt.Errorf("%d question(s) failed", failed)
	}
	return nil
}

func newProof(q *catalog.Question) *questionProof {
	return &questionProof{ID: q.ID, Track: q.Track, Domain: q.Domain, Difficulty: q.Difficulty, Environment: q.Environment,
		Disruptive: q.Disruptive, Fresh: q.FreshEnvironment, Checks: len(q.Checks), Points: q.Points}
}

// proveQuestion runs the reference solution opts.repeat times and then every
// variant once. The first run that is not ok decides the status of the question.
func proveQuestion(ctx context.Context, mgr *session.Manager, sid string, q *catalog.Question, opts proofOptions) *questionProof {
	start := time.Now()
	qp := newProof(q)
	qp.Status = statusOK
	add := func(r *proofRun) bool {
		qp.Runs = append(qp.Runs, r)
		if r.Status != statusOK && qp.Status == statusOK {
			qp.Status = r.Status
			qp.Reason = r.Reason
			if r.Script != "solution.sh" || opts.repeat > 1 {
				qp.Reason = fmt.Sprintf("%s (run %d): %s", r.Script, r.Run, r.Reason)
			}
		}
		return r.Status == statusOK && ctx.Err() == nil
	}
	ok := true
	for n := 1; n <= opts.repeat && ok; n++ {
		ok = add(proveRun(ctx, mgr, sid, q, n, "solution.sh", q.SolutionSh, nil, opts.settle, opts.quick))
	}
	if opts.variants {
		for _, v := range q.Variants {
			if !ok {
				break
			}
			var fails []string
			if !v.Pass {
				fails = v.Fails
			}
			ok = add(proveRun(ctx, mgr, sid, q, 1, "variants/"+v.Name+".sh", v.Script, fails, opts.settle, opts.quick))
		}
	}
	qp.Seconds = time.Since(start).Seconds()
	return qp
}

func waitReady(ctx context.Context, mgr *session.Manager, sid, qid string) error {
	deadline := time.Now().Add(15 * time.Minute)
	for {
		v, err := mgr.View(sid)
		if err != nil {
			return err
		}
		var status, qerr string
		for _, x := range v.Questions {
			if x.ID == qid {
				status, qerr = x.Status, x.Error
			}
		}
		if status == "ready" {
			return nil
		}
		if status == "error" {
			return fmt.Errorf("setup: %s", qerr)
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return fmt.Errorf("setup timed out")
		}
		time.Sleep(time.Second)
	}
}

// proveRun resets the question, grades the untouched state, applies script
// and grades again. mustFail lists the checks the script is expected to leave
// failing (empty for a correct solution).
func proveRun(ctx context.Context, mgr *session.Manager, sid string, q *catalog.Question, n int, name, script string, mustFail []string, settle time.Duration, quick bool) *proofRun {
	r := &proofRun{Run: n, Script: name, Expect: "pass", Status: statusOK}
	if len(mustFail) > 0 {
		r.Expect = "fail " + strings.Join(mustFail, " ")
	}
	stop := func(status, format string, a ...any) *proofRun {
		r.Status, r.Reason = status, fmt.Sprintf(format, a...)
		return r
	}
	if err := mgr.Prepare(sid, q.ID, true); err != nil {
		return stop(statusSetupError, "%v", err)
	}
	if err := waitReady(ctx, mgr, sid, q.ID); err != nil {
		return stop(statusSetupError, "%v", err)
	}
	// With quick, a check that is expected to fail gets one look: on the
	// untouched state that is every check, afterwards those in mustFail.
	var untouched, expectedFail func(*catalog.Check) bool
	if quick {
		untouched = func(*catalog.Check) bool { return true }
		expectedFail = func(c *catalog.Check) bool { return slices.Contains(mustFail, c.ID) }
	}
	before, err := mgr.CheckOnce(ctx, sid, q.ID, untouched)
	if err != nil {
		return stop(statusSetupError, "grading the untouched state: %v", err)
	}
	r.Before = before
	if free := unsolvedPasses(before); len(free) > 0 {
		return stop(statusFalsePositive, "passes before the task is solved: %s (fix the check, or mark it guard: true if it protects against a shortcut)", strings.Join(free, ", "))
	}
	res, err := mgr.Solve(ctx, sid, q.ID, script)
	if err != nil {
		return stop(statusSolutionError, "%s: %v", name, err)
	}
	if res.ExitCode != 0 {
		return stop(statusSolutionError, "%s exited %d: %s", name, res.ExitCode, tail(checker.Redact(res.Stderr+res.Stdout), 300))
	}
	// Grade until the outcome is the declared one or the settle time is over.
	t0 := time.Now()
	end := t0.Add(settle)
	var after *checker.Result
	for {
		after, err = mgr.CheckOnce(ctx, sid, q.ID, expectedFail)
		if err != nil {
			return stop(statusSetupError, "grading: %v", err)
		}
		r.After = after
		if len(mismatch(after, mustFail)) == 0 || time.Now().After(end) || ctx.Err() != nil {
			break
		}
		time.Sleep(4 * time.Second)
	}
	r.SettledSeconds = time.Since(t0).Seconds()
	if bad := mismatch(after, mustFail); len(bad) > 0 {
		status := statusFalseNegative
		for _, b := range bad {
			if b.wantFail {
				status = statusFalsePositive
			}
		}
		return stop(status, "%d/%d after %s: %s", after.Score, after.Max, name, describeMismatch(bad))
	}
	// The same state must be graded the same way again.
	for i := 0; i < 2; i++ {
		time.Sleep(2 * time.Second)
		again, err := mgr.CheckOnce(ctx, sid, q.ID, expectedFail)
		if err != nil {
			return stop(statusSetupError, "grading again: %v", err)
		}
		r.Rechecks++
		if bad := mismatch(again, mustFail); len(bad) > 0 {
			r.After = again
			return stop(statusFlaky, "graded differently on re-check %d without any change: %s", i+1, describeMismatch(bad))
		}
	}
	return r
}

// unsolvedPasses returns the checks that pass although nothing was done and
// that are not declared as guards.
func unsolvedPasses(res *checker.Result) []string {
	var out []string
	for _, c := range res.Checks {
		if c.Passed && !c.Guard {
			out = append(out, c.ID)
		}
	}
	return out
}

type checkMismatch struct {
	check    checker.CheckResult
	wantFail bool
}

// mismatch compares a grading result with the declared outcome: the checks in
// mustFail must fail and all others must pass.
func mismatch(res *checker.Result, mustFail []string) []checkMismatch {
	fail := map[string]bool{}
	for _, id := range mustFail {
		fail[id] = true
	}
	var out []checkMismatch
	for _, c := range res.Checks {
		if c.Passed == fail[c.ID] {
			out = append(out, checkMismatch{check: c, wantFail: fail[c.ID]})
		}
	}
	return out
}

func describeMismatch(bad []checkMismatch) string {
	var parts []string
	for _, b := range bad {
		if b.wantFail {
			parts = append(parts, fmt.Sprintf("[%s] passes but must fail", b.check.ID))
			continue
		}
		parts = append(parts, fmt.Sprintf("[%s] want %s, got %s", b.check.ID, b.check.Expected, strings.ReplaceAll(b.check.Observed, "\n", " ")))
	}
	return strings.Join(parts, "; ")
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return "…" + s[len(s)-n:]
	}
	return s
}

// testExam sets up every question of a track in one exam session, applies all
// reference solutions and expects a perfect score. It proves that questions
// do not disturb each other when they share clusters, as they do in an exam,
// and it exercises the exam's own grading path (Finish).
func testExam(ctx context.Context, mgr *session.Manager, t *catalog.Track, keep bool) *examProof {
	start := time.Now()
	ep := &examProof{Track: t.ID, Status: statusOK}
	stop := func(status string, err error) *examProof {
		ep.Status, ep.Reason, ep.Seconds = status, err.Error(), time.Since(start).Seconds()
		return ep
	}
	ses, err := mgr.NewExam(t.ID, session.ExamOptions{Questions: len(t.Questions), Minutes: 600})
	if err != nil {
		return stop(statusSetupError, err)
	}
	fmt.Printf("\n%s — full exam with %d questions (session %s)\n", t.Title, len(ses.Questions), ses.ID)
	if !keep {
		defer func() {
			dctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			_ = mgr.Destroy(dctx, ses.ID)
		}()
	}
	wait := func(want string, max time.Duration) (*session.Session, error) {
		end := time.Now().Add(max)
		for {
			v, err := mgr.View(ses.ID)
			if err != nil {
				return nil, err
			}
			if v.Status == want {
				return v, nil
			}
			if v.Status == session.StatusError {
				return nil, fmt.Errorf("session failed: %s", v.Error)
			}
			if time.Now().After(end) || ctx.Err() != nil {
				return nil, fmt.Errorf("timed out waiting for %s (status %s)", want, v.Status)
			}
			time.Sleep(2 * time.Second)
		}
	}
	v, err := wait(session.StatusReady, 40*time.Minute)
	if err != nil {
		return stop(statusSetupError, err)
	}
	fmt.Printf("  provisioned in %s\n", time.Since(start).Round(time.Second))
	if err := mgr.Start(ses.ID); err != nil {
		return stop(statusSetupError, err)
	}
	var broken []string
	for _, qs := range v.Questions {
		q := t.Question(qs.ID)
		res, err := mgr.Solve(ctx, ses.ID, q.ID, q.SolutionSh)
		if err != nil {
			return stop(statusSolutionError, err)
		}
		if res.ExitCode != 0 {
			fmt.Printf("  ! solution of %s exited %d: %s\n", q.ID, res.ExitCode, tail(checker.Redact(res.Stderr+res.Stdout), 200))
			broken = append(broken, q.ID)
		}
	}
	// No fixed sleep: exam grading re-runs failed checks for the settle time.
	fmt.Println("  solutions applied, grading")
	if err := mgr.Finish(ctx, ses.ID); err != nil {
		return stop(statusSetupError, err)
	}
	v, err = wait(session.StatusFinished, 30*time.Minute)
	if err != nil {
		return stop(statusSetupError, err)
	}
	var bad []string
	for _, qs := range v.Questions {
		if qs.Last == nil {
			bad = append(bad, qs.ID)
			fmt.Printf("  ✗ %-40s was not graded\n", qs.ID)
			continue
		}
		ep.Questions = append(ep.Questions, qs.Last)
		if qs.Last.Passed {
			continue
		}
		bad = append(bad, qs.ID)
		for _, c := range qs.Last.Checks {
			if !c.Passed {
				fmt.Printf("  ✗ %-40s [%s] want %s, got %s\n", qs.ID, c.ID, c.Expected, strings.ReplaceAll(c.Observed, "\n", " "))
			}
		}
	}
	if v.Report != nil {
		ep.Score, ep.Max = v.Report.Score, v.Report.Max
		fmt.Printf("  score %d/%d (%.1f%%)\n", v.Report.Score, v.Report.Max, v.Report.Percent)
	}
	ep.Seconds = time.Since(start).Seconds()
	sort.Strings(bad)
	switch {
	case len(broken) > 0:
		ep.Status, ep.Reason = statusSolutionError, "solution.sh failed in the shared exam clusters: "+strings.Join(broken, ", ")
	case len(bad) > 0:
		ep.Status, ep.Reason = statusFalseNegative, fmt.Sprintf("%d question(s) did not reach full points together: %s", len(bad), strings.Join(bad, ", "))
	default:
		fmt.Printf("  ✓ all %d questions passed together\n", len(v.Questions))
	}
	return ep
}
