package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/kubedojo/kubedojo/internal/catalog"
	"github.com/kubedojo/kubedojo/internal/session"
	"github.com/kubedojo/kubedojo/internal/store"
)

// cmdTest proves each question end to end against a real sandbox:
//  1. setup.sh runs,
//  2. the checks do NOT all pass on the untouched environment (they are sharp),
//  3. solution.sh runs,
//  4. every check passes (the question is solvable and the checks are fair).
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
	if err := fl.Parse(args); err != nil {
		return err
	}
	cat, err := catalog.Load(c.contentFS())
	if err != nil {
		return fmt.Errorf("content is invalid:\n%w", err)
	}
	drv := c.driver()
	if err := preflight(ctx, drv, true); err != nil {
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
	mgr, err := session.NewManager(session.Config{Catalog: cat, Store: st, Driver: drv, Logger: log, MaxSessions: 10})
	if err != nil {
		return err
	}
	defer mgr.Close()

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
			if err := testExam(ctx, mgr, t, *settle, *keep); err != nil {
				fmt.Printf("  ✗ %s: %v\n", t.ID, err)
				bad++
			}
		}
		if bad > 0 {
			return fmt.Errorf("%d exam(s) failed", bad)
		}
		return nil
	}
	failed, total := 0, 0
	for _, t := range cat.List() {
		if *track != "" && t.ID != *track {
			continue
		}
		var qs []*catalog.Question
		for _, q := range t.Questions {
			if (len(want) == 0 || want[q.ID]) && q.Driver == drv.Name() {
				qs = append(qs, q)
			}
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
			start := time.Now()
			err := testQuestion(ctx, mgr, ses.ID, q, *settle)
			if err != nil {
				failed++
				fmt.Printf("  ✗ %-44s %5.0fs  %v\n", q.ID, time.Since(start).Seconds(), err)
			} else {
				fmt.Printf("  ✓ %-44s %5.0fs\n", q.ID, time.Since(start).Seconds())
			}
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
	fmt.Printf("\n%d of %d questions passed\n", total-failed, total)
	if failed > 0 {
		return fmt.Errorf("%d question(s) failed", failed)
	}
	return nil
}

func testQuestion(ctx context.Context, mgr *session.Manager, sid string, q *catalog.Question, settle time.Duration) error {
	if err := mgr.Prepare(sid, q.ID, true); err != nil {
		return err
	}
	deadline := time.Now().Add(15 * time.Minute)
	for {
		v, err := mgr.View(sid)
		if err != nil {
			return err
		}
		var status, qerr string
		for _, x := range v.Questions {
			if x.ID == q.ID {
				status, qerr = x.Status, x.Error
			}
		}
		if status == "ready" {
			break
		}
		if status == "error" {
			return fmt.Errorf("setup: %s", qerr)
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return fmt.Errorf("setup timed out")
		}
		time.Sleep(time.Second)
	}
	before, err := mgr.Check(ctx, sid, q.ID)
	if err != nil {
		return err
	}
	if before.Passed {
		return fmt.Errorf("checks already pass before solving — they do not test anything")
	}
	res, err := mgr.Exec(ctx, sid, q.ID, "control-plane", q.SolutionSh)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("solution.sh exited %d: %s", res.ExitCode, tail(res.Stderr+res.Stdout, 300))
	}
	end := time.Now().Add(settle)
	for {
		after, err := mgr.Check(ctx, sid, q.ID)
		if err != nil {
			return err
		}
		if after.Passed {
			return nil
		}
		if time.Now().After(end) || ctx.Err() != nil {
			var bad []string
			for _, c := range after.Checks {
				if !c.Passed {
					bad = append(bad, fmt.Sprintf("[%s] want %s, got %s", c.ID, c.Expected, strings.ReplaceAll(c.Observed, "\n", " ")))
				}
			}
			return fmt.Errorf("after solution %d/%d: %s", after.Score, after.Max, strings.Join(bad, "; "))
		}
		time.Sleep(4 * time.Second)
	}
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
// do not disturb each other when they share clusters, as they do in an exam.
func testExam(ctx context.Context, mgr *session.Manager, t *catalog.Track, settle time.Duration, keep bool) error {
	ses, err := mgr.NewExam(t.ID, session.ExamOptions{Questions: len(t.Questions), Minutes: 600})
	if err != nil {
		return err
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
	start := time.Now()
	v, err := wait(session.StatusReady, 40*time.Minute)
	if err != nil {
		return err
	}
	fmt.Printf("  provisioned in %s\n", time.Since(start).Round(time.Second))
	if err := mgr.Start(ses.ID); err != nil {
		return err
	}
	for _, qs := range v.Questions {
		q := t.Question(qs.ID)
		res, err := mgr.Exec(ctx, ses.ID, q.ID, "control-plane", q.SolutionSh)
		if err != nil {
			return err
		}
		if res.ExitCode != 0 {
			fmt.Printf("  ! solution of %s exited %d: %s\n", q.ID, res.ExitCode, tail(res.Stderr+res.Stdout, 200))
		}
	}
	fmt.Printf("  solutions applied, settling for %s\n", settle)
	select {
	case <-time.After(settle):
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := mgr.Finish(ctx, ses.ID); err != nil {
		return err
	}
	v, err = wait(session.StatusFinished, 30*time.Minute)
	if err != nil {
		return err
	}
	bad := 0
	for _, qs := range v.Questions {
		if qs.Last == nil || !qs.Last.Passed {
			bad++
			for _, c := range qs.Last.Checks {
				if !c.Passed {
					fmt.Printf("  ✗ %-40s [%s] want %s, got %s\n", qs.ID, c.ID, c.Expected, strings.ReplaceAll(c.Observed, "\n", " "))
				}
			}
		}
	}
	fmt.Printf("  score %d/%d (%.1f%%)\n", v.Report.Score, v.Report.Max, v.Report.Percent)
	if bad > 0 {
		return fmt.Errorf("%d question(s) did not reach full points", bad)
	}
	fmt.Printf("  ✓ all %d questions passed together\n", len(v.Questions))
	return nil
}
