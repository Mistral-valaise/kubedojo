package checker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"go.opentelemetry.io/otel/codes"

	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox"
	"github.com/Mistral-valaise/kubedojo/internal/telemetry/teltest"
)

// Every check is one span below the span of its question. The span says how
// the check ended; only a check that could not be evaluated is an error.
func TestEveryCheckIsASpanWithItsOutcome(t *testing.T) {
	rec := teltest.New()
	const learner = "hunter2-written-by-the-learner"
	q := &catalog.Question{ID: "q", Points: 7, Checks: []*catalog.Check{
		chk("good", &catalog.Expect{Equals: ptr("ok")}),
		{ID: "wrong", Title: "wrong", Points: 2, Type: "exec", Run: "wrong", On: "worker", Expect: &catalog.Expect{Equals: ptr("ok")}},
		{ID: "slow", Title: "slow", Points: 1, Type: "exec", Run: "slow", TimeoutD: 20 * time.Millisecond},
		{ID: "broken", Title: "broken", Points: 1, Type: "exec", Run: "broken"},
		{ID: "kept", Title: "kept", Points: 2, Type: "exec", Run: "kept", Guard: true, Expect: &catalog.Expect{ExitCode: ptr(3)}},
	}}
	run := func(ctx context.Context, _, script string) (sandbox.ExecResult, error) {
		switch strings.TrimSpace(script) {
		case "good":
			return sandbox.ExecResult{Stdout: "ok"}, nil
		case "wrong":
			return sandbox.ExecResult{Stdout: learner, Stderr: learner, ExitCode: 0}, nil
		case "slow":
			<-ctx.Done()
			return sandbox.ExecResult{ExitCode: -1}, ctx.Err()
		case "kept":
			return sandbox.ExecResult{ExitCode: 3}, nil
		}
		return sandbox.ExecResult{}, errors.New("cannot reach the sandbox")
	}

	ctx, parent := rec.Start(context.Background(), "question.check")
	res := Run(ctx, q, run)
	parent.End(nil)

	spans := rec.Named("check.run")
	if len(spans) != len(q.Checks) {
		t.Fatalf("%d check.run spans for %d checks:\n%s", len(spans), len(q.Checks), teltest.Tree(rec.Ended()))
	}
	question := rec.Named("question.check")[0]
	want := []struct {
		id, outcome, target string
		passed, timedOut    bool
		exit, points        int64
		guard               bool
		status              codes.Code
	}{
		{id: "good", outcome: OutcomePassed, target: "control-plane", passed: true, points: 1},
		{id: "wrong", outcome: OutcomeFailed, target: "worker", points: 2},
		{id: "slow", outcome: OutcomeTimedOut, target: "control-plane", timedOut: true, exit: -1, points: 1},
		{id: "broken", outcome: OutcomeGraderError, target: "control-plane", points: 1, status: codes.Error},
		{id: "kept", outcome: OutcomePassed, target: "control-plane", passed: true, exit: 3, points: 2, guard: true},
	}
	for i, w := range want {
		s := spans[i]
		a := teltest.Attrs(s)
		if s.Parent().SpanID() != question.SpanContext().SpanID() {
			t.Errorf("%s: the span is not a child of question.check", w.id)
		}
		if a["kubedojo.check.id"].AsString() != w.id || a["kubedojo.check.outcome"].AsString() != w.outcome ||
			a["kubedojo.check.passed"].AsBool() != w.passed || a["kubedojo.check.timed_out"].AsBool() != w.timedOut ||
			a["kubedojo.check.attempts"].AsInt64() != 1 || a["kubedojo.check.points"].AsInt64() != w.points ||
			a["kubedojo.check.guard"].AsBool() != w.guard || a["kubedojo.exec.target"].AsString() != w.target ||
			a["process.exit.code"].AsInt64() != w.exit || a["kubedojo.check.command"].AsString() != w.id {
			t.Errorf("%s: attributes %v", w.id, s.Attributes())
		}
		if s.Status().Code != w.status {
			t.Errorf("%s: status %v, want %v", w.id, s.Status().Code, w.status)
		}
		if got := res.Checks[i].Outcome(); got != w.outcome {
			t.Errorf("%s: Outcome() = %q, want %q", w.id, got, w.outcome)
		}
		// One attempt, one event.
		if ev := teltest.Events(s, "attempt"); len(ev) != 1 || ev[0]["n"].AsInt64() != 1 || ev[0]["passed"].AsBool() != w.passed || ev[0]["exit_code"].AsInt64() != w.exit {
			t.Errorf("%s: attempt events %v", w.id, ev)
		}
		// What the command printed can quote the learner. It is in the API
		// response, never in the trace.
		for k, v := range a {
			if strings.Contains(v.String(), learner) {
				t.Errorf("%s: attribute %s carries the output of the command", w.id, k)
			}
		}
	}
	broken := teltest.Attrs(spans[3])
	if broken["error.type"].AsString() != OutcomeGraderError || spans[3].Status().Description != "cannot reach the sandbox" {
		t.Errorf("grader error: error.type %q, status %q", broken["error.type"].AsString(), spans[3].Status().Description)
	}
	if _, ok := teltest.Attrs(spans[1])["error.type"]; ok {
		t.Error("a failed check carries error.type: a wrong answer is no error")
	}
	if n := res.GraderErrors(); n != 1 {
		t.Errorf("GraderErrors() = %d, want 1", n)
	}
}

// A retried check stays one span. Its attempts are events: the first five and
// the last, so that a check retried for minutes does not fill the span.
func TestRetriesAreEventsOfOneSpan(t *testing.T) {
	defer func(d time.Duration) { retryPause = d }(retryPause)
	retryPause = time.Millisecond
	rec := teltest.New()
	c := chk("late", &catalog.Expect{Equals: ptr("ok")})
	c.RetryD = time.Minute
	n := 0
	run := func(context.Context, string, string) (sandbox.ExecResult, error) {
		if n++; n < 9 {
			return sandbox.ExecResult{Stdout: "no", ExitCode: 0}, nil
		}
		return sandbox.ExecResult{Stdout: "ok"}, nil
	}
	res := Run(span0(rec), &catalog.Question{ID: "q", Points: 1, Checks: []*catalog.Check{c}}, run)
	if !res.Passed || res.Checks[0].Attempts != 9 {
		t.Fatalf("passed=%v attempts=%d, want true 9", res.Passed, res.Checks[0].Attempts)
	}
	spans := rec.Named("check.run")
	if len(spans) != 1 {
		t.Fatalf("%d spans for one retried check", len(spans))
	}
	a := teltest.Attrs(spans[0])
	if a["kubedojo.check.attempts"].AsInt64() != 9 || !a["kubedojo.check.passed"].AsBool() || a["kubedojo.check.outcome"].AsString() != OutcomePassed {
		t.Errorf("attributes %v", spans[0].Attributes())
	}
	var got []int64
	for _, e := range teltest.Events(spans[0], "attempt") {
		got = append(got, e["n"].AsInt64())
		if e["passed"].AsBool() != (e["n"].AsInt64() == 9) {
			t.Errorf("attempt %d: passed=%v", e["n"].AsInt64(), e["passed"].AsBool())
		}
	}
	if want := []int64{1, 2, 3, 4, 5, 9}; len(got) != len(want) {
		t.Fatalf("attempt events %v, want %v", got, want)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("attempt events %v, want %v", got, want)
			}
		}
	}
}

// span0 is a context with a tracer and no span.
func span0(rec *teltest.Recorder) context.Context {
	ctx, s := rec.Start(context.Background(), "root")
	s.End(nil)
	return ctx
}

// Looking again at failed checks, as exam grading does every few seconds,
// adds no spans: the first look is in the trace, the rounds are the caller's.
func TestRecheckStartsNoSpans(t *testing.T) {
	rec := teltest.New()
	q := &catalog.Question{ID: "q", Points: 1, Checks: []*catalog.Check{chk("wrong", &catalog.Expect{Equals: ptr("ok")})}}
	out := "no"
	run := func(context.Context, string, string) (sandbox.ExecResult, error) {
		return sandbox.ExecResult{Stdout: out}, nil
	}
	ctx := span0(rec)
	res := Run(ctx, q, run)
	out = "ok"
	for i := 0; i < 3; i++ {
		res = Recheck(ctx, q, run, res)
	}
	if !res.Passed || res.Checks[0].Attempts != 2 {
		t.Fatalf("passed=%v attempts=%d, want true 2", res.Passed, res.Checks[0].Attempts)
	}
	if n := len(rec.Named("check.run")); n != 1 {
		t.Errorf("%d check.run spans after three re-checks, want the one of the first look", n)
	}
}

// The command of a check is written by the content author, so it may be on
// the span. A long one is clipped, never in the middle of a character.
func TestTheCommandOnTheSpanIsClipped(t *testing.T) {
	rec := teltest.New()
	long := strings.Repeat("é", 2*commandAttrLimit)
	q := &catalog.Question{ID: "q", Points: 1, Checks: []*catalog.Check{{ID: "c", Points: 1, Type: "exec", Run: long}}}
	Run(span0(rec), q, func(context.Context, string, string) (sandbox.ExecResult, error) { return sandbox.ExecResult{}, nil })
	cmd := teltest.Attrs(rec.Named("check.run")[0])["kubedojo.check.command"].AsString()
	if !utf8.ValidString(cmd) || utf8.RuneCountInString(cmd) > commandAttrLimit+2 || !strings.HasPrefix(cmd, "ééé") {
		t.Errorf("the command has %d characters (valid: %v), want at most %d", utf8.RuneCountInString(cmd), utf8.ValidString(cmd), commandAttrLimit+2)
	}
}

// Without a tracer in the context nothing is traced and nothing breaks.
func TestRunWithoutATracer(t *testing.T) {
	q := &catalog.Question{ID: "q", Points: 1, Checks: []*catalog.Check{chk("c", nil)}}
	res := Run(context.Background(), q, func(context.Context, string, string) (sandbox.ExecResult, error) { return sandbox.ExecResult{}, nil })
	if !res.Passed {
		t.Errorf("result %+v", res)
	}
}
