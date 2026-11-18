package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/session"
)

// needsSandbox reports whether proving the selected tracks takes Docker.
// Theory tracks are graded from the catalog alone.
func needsSandbox(cat *catalog.Catalog, only string) bool {
	for _, t := range cat.List() {
		if (only == "" || t.ID == only) && !t.Sandboxless() {
			return true
		}
	}
	return false
}

// proveChoice proves the grading of one multiple-choice question through the
// same session calls the UI makes:
//  1. the options are shown in an order that contains each of them once,
//  2. no answer scores 0,
//  3. the correct options score full points,
//  4. every option picked alone scores 0 unless it is the whole answer,
//  5. the correct options plus one wrong option score 0.
//
// It cannot prove that the option marked correct is the true one: that takes
// a reviewer and the question's references.
func proveChoice(mgr *session.Manager, sid string, q *catalog.Question) *questionProof {
	start := time.Now()
	qp := newProof(q)
	qp.Status = statusOK
	correct := q.Choice.Correct()
	try := func(name string, answer []string, want bool) bool {
		r := &proofRun{Run: len(qp.Runs) + 1, Script: name, Expect: "fail", Status: statusOK}
		if want {
			r.Expect = "pass"
		}
		qp.Runs = append(qp.Runs, r)
		stop := func(status, format string, a ...any) bool {
			r.Status, r.Reason = status, fmt.Sprintf(format, a...)
			qp.Status, qp.Reason = status, name+": "+r.Reason
			return false
		}
		if err := mgr.Prepare(sid, q.ID, true); err != nil {
			return stop(statusSetupError, "%v", err)
		}
		v, err := mgr.View(sid)
		if err != nil {
			return stop(statusSetupError, "%v", err)
		}
		if order := shownOrder(v, q.ID); !sameIDs(order, q.Choice) {
			return stop(statusSetupError, "the session shows options %v, the question has %d others", order, len(q.Choice.Options))
		}
		if err := mgr.Answer(sid, q.ID, answer); err != nil {
			return stop(statusSetupError, "answer refused: %v", err)
		}
		res, err := mgr.Check(context.Background(), sid, q.ID)
		if err != nil {
			return stop(statusSetupError, "grading: %v", err)
		}
		r.After = res
		switch {
		case want && (!res.Passed || res.Score != q.Points || res.Max != q.Points):
			return stop(statusFalseNegative, "scores %d/%d, want %d/%d", res.Score, res.Max, q.Points, q.Points)
		case !want && (res.Passed || res.Score != 0):
			return stop(statusFalsePositive, "scores %d/%d, want 0", res.Score, res.Max)
		}
		return true
	}
	ok := try("no answer", nil, false) && try("correct answer", correct, true)
	for _, o := range q.Choice.Options {
		if !ok {
			break
		}
		alone := []string{o.ID}
		ok = try(fmt.Sprintf("only %q", o.Text), alone, slices.Equal(alone, correct))
		if ok && !o.Correct && q.Choice.Multiple {
			ok = try(fmt.Sprintf("correct answer plus %q", o.Text), append(slices.Clone(correct), o.ID), false)
		}
	}
	qp.Seconds = time.Since(start).Seconds()
	return qp
}

func shownOrder(v *session.Session, qid string) []string {
	for _, qs := range v.Questions {
		if qs.ID == qid {
			return qs.Order
		}
	}
	return nil
}

// sameIDs reports whether order names every option of c exactly once.
func sameIDs(order []string, c *catalog.Choice) bool {
	want := make([]string, len(c.Options))
	for i, o := range c.Options {
		want[i] = o.ID
	}
	got := slices.Clone(order)
	slices.Sort(got)
	slices.Sort(want)
	return slices.Equal(got, want)
}

// testQuizExam proves the exam path of a theory track twice with every
// question of the track: answered correctly the exam must score 100%, answered
// wrongly 0%. That covers what the per-question proof does not: autosaved
// answers, grading at the end, and the report.
func testQuizExam(mgr *session.Manager, t *catalog.Track) *examProof {
	start := time.Now()
	ep := &examProof{Track: t.ID, Status: statusOK}
	fmt.Printf("\n%s — full exam with %d questions, no sandbox\n", t.Title, len(t.Questions))
	run := func(name string, pick func(*catalog.Question) []string, wantPercent float64) (*session.Session, error) {
		ses, err := mgr.NewExam(t.ID, session.ExamOptions{Questions: len(t.Questions), Minutes: 600})
		if err != nil {
			return nil, err
		}
		defer func() { _ = mgr.Destroy(context.Background(), ses.ID) }()
		if len(ses.Questions) != len(t.Questions) {
			return nil, fmt.Errorf("the exam has %d of %d questions", len(ses.Questions), len(t.Questions))
		}
		if err := mgr.Start(ses.ID); err != nil {
			return nil, err
		}
		for _, q := range t.Questions {
			if err := mgr.Answer(ses.ID, q.ID, pick(q)); err != nil {
				return nil, fmt.Errorf("%s: answer refused: %w", q.ID, err)
			}
		}
		if err := mgr.Finish(context.Background(), ses.ID); err != nil {
			return nil, err
		}
		v, err := mgr.View(ses.ID)
		if err != nil {
			return nil, err
		}
		if v.Status != session.StatusFinished || v.Report == nil {
			return nil, fmt.Errorf("the exam is %s after Finish returned, want finished with a report", v.Status)
		}
		var off []string
		for _, qs := range v.Questions {
			if qs.Last == nil || qs.Last.Passed != (wantPercent == 100) {
				off = append(off, qs.ID)
			}
		}
		fmt.Printf("  %s: score %d/%d (%.1f%%)\n", name, v.Report.Score, v.Report.Max, v.Report.Percent)
		if v.Report.Percent != wantPercent || v.Report.Max != len(t.Questions) || len(off) > 0 {
			return v, fmt.Errorf("%s: %d/%d (%.1f%%), want %.0f%% of %d; graded unexpectedly: %s", name,
				v.Report.Score, v.Report.Max, v.Report.Percent, wantPercent, len(t.Questions), strings.Join(off, ", "))
		}
		return v, nil
	}
	firstWrong := func(q *catalog.Question) []string {
		for _, o := range q.Choice.Options {
			if !o.Correct {
				return []string{o.ID}
			}
		}
		return nil
	}
	v, err := run("every question answered correctly", func(q *catalog.Question) []string { return q.Choice.Correct() }, 100)
	if v != nil {
		ep.Score, ep.Max = v.Report.Score, v.Report.Max
		for _, qs := range v.Questions {
			ep.Questions = append(ep.Questions, qs.Last)
		}
	}
	// A run that could not be graded at all returns no session.
	fail := func(v *session.Session, graded string, err error) *examProof {
		ep.Status, ep.Reason, ep.Seconds = graded, err.Error(), time.Since(start).Seconds()
		if v == nil {
			ep.Status = statusSetupError
		}
		return ep
	}
	if err != nil {
		return fail(v, statusFalseNegative, err)
	}
	if v, err := run("every question answered wrongly", firstWrong, 0); err != nil {
		return fail(v, statusFalsePositive, err)
	}
	ep.Seconds = time.Since(start).Seconds()
	fmt.Printf("  ✓ all %d questions graded as answered\n", len(t.Questions))
	return ep
}
