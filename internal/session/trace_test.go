package session_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"go.opentelemetry.io/otel/codes"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/checker"
	"github.com/Mistral-valaise/kubedojo/internal/dockerhost"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox/sandboxtest"
	"github.com/Mistral-valaise/kubedojo/internal/session"
	"github.com/Mistral-valaise/kubedojo/internal/span"
	"github.com/Mistral-valaise/kubedojo/internal/store"
	"github.com/Mistral-valaise/kubedojo/internal/telemetry"
	"github.com/Mistral-valaise/kubedojo/internal/telemetry/teltest"
)

// The trace of session work, recorded with the fake driver. The trees these
// tests log (go test -v -run Trace ./internal/session/) are what a person
// sees in Tempo, minus the spans of the real driver below sandbox.env.create.

const (
	tracedQuestions = 17 // a full exam
	tracedChecks    = 5
)

// tracedCatalog has one track on a cluster: 17 questions with five checks
// each, every one with a setup and a cleanup script.
func tracedCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	fsys := fstest.MapFS{}
	add := func(name, body string) { fsys[name] = &fstest.MapFile{Data: []byte(body)} }
	add("x/track.yaml", fmt.Sprintf("title: X\nkubernetesVersion: v1.37\nexam: {durationMinutes: 10, passPercent: 50, questions: %d}\n", tracedQuestions)+
		"domains: [{id: d, title: D, weight: 100}]\nenvironments: {base: {workers: 1}}\n")
	add("x/lessons/d.md", "# D")
	for i := 1; i <= tracedQuestions; i++ {
		id := fmt.Sprintf("q%02d", i)
		var checks strings.Builder
		for c := 1; c <= tracedChecks; c++ {
			fmt.Fprintf(&checks, "  - {id: c%d, title: T, type: exec, run: check c%d, explain: e}\n", c, c)
		}
		dir := "x/questions/" + id + "/"
		add(dir+"question.yaml", "title: Q "+id+"\ndomain: d\ntask: do\nhints: [h]\nchecks:\n"+checks.String())
		add(dir+"setup.sh", "setup")
		add(dir+"cleanup.sh", "cleanup")
		add(dir+"solution.sh", "solution")
		add(dir+"solution.md", "do it")
	}
	cat, err := catalog.Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

// scripted is the fake driver with commands that can fail: answer decides
// how the script of a question ends.
type scripted struct {
	sandboxtest.Driver
	mu     sync.Mutex
	answer func(question, script string) (sandbox.ExecResult, error)
}

func (d *scripted) Exec(ctx context.Context, h dockerhost.Host, sid string, t sandbox.Target, user, script string, env map[string]string) (sandbox.ExecResult, error) {
	_, _ = d.Driver.Exec(ctx, h, sid, t, user, script, env)
	d.mu.Lock()
	answer := d.answer
	d.mu.Unlock()
	if answer == nil {
		return sandbox.ExecResult{}, nil
	}
	return answer(env["KD_QUESTION"], strings.TrimSpace(script))
}

func (d *scripted) answers(fn func(question, script string) (sandbox.ExecResult, error)) {
	d.mu.Lock()
	d.answer = fn
	d.mu.Unlock()
}

type traced struct {
	mgr  *session.Manager
	rec  *teltest.Recorder
	drv  *scripted
	logs *lockedBuffer
	// What the hooks were told.
	mu        sync.Mutex
	terminals []time.Duration
	storeOps  map[string]int
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

// lines returns the log lines written so far, each as its JSON object.
func (l *lockedBuffer) lines(t *testing.T) []map[string]any {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []map[string]any
	for _, raw := range bytes.Split(bytes.TrimSpace(l.b.Bytes()), []byte("\n")) {
		if len(raw) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		out = append(out, m)
	}
	return out
}

// newTraced wires a manager to a recording tracer the way the server wires
// it to telemetry.
func newTraced(t *testing.T, cat *catalog.Catalog, settle time.Duration) *traced {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	x := &traced{drv: &scripted{}, logs: &lockedBuffer{}, storeOps: map[string]int{}}
	x.rec = teltest.NewWith(noopmetric.NewMeterProvider(), slog.New(slog.NewJSONHandler(x.logs, nil)))
	x.mgr, err = session.NewManager(session.Config{Catalog: cat, Store: st, Driver: x.drv, Logger: x.rec.Logger, GradeSettle: settle,
		Hooks: session.Hooks{
			Tracer: x.rec,
			Detach: telemetry.Detach,
			TerminalClosed: func(_ context.Context, d time.Duration) {
				x.mu.Lock()
				x.terminals = append(x.terminals, d)
				x.mu.Unlock()
			},
			StoreOperation: func(op string, _ time.Duration) {
				x.mu.Lock()
				x.storeOps[op]++
				x.mu.Unlock()
			},
		}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(x.mgr.Close)
	return x
}

// request runs fn as an API request would: inside a span whose context ends
// when the request is answered.
func (x *traced) request(name string, fn func(ctx context.Context)) sdktrace.ReadOnlySpan {
	ctx, cancel := context.WithCancel(context.Background())
	ctx, s := x.rec.Start(ctx, name)
	fn(ctx)
	s.End(nil)
	cancel()
	return x.rec.Named(name)[len(x.rec.Named(name))-1]
}

// ended waits until n spans with the name have ended and returns them.
func (x *traced) ended(t *testing.T, name string, n int) []sdktrace.ReadOnlySpan {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(2 * time.Millisecond) {
		if got := x.rec.Named(name); len(got) >= n {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d spans named %q ended, want %d:\n%s", len(x.rec.Named(name)), name, n, teltest.Shape(x.rec.Ended()))
		}
	}
}

// trace returns the spans of the trace of a request, once the named span of
// it has ended.
func (x *traced) trace(t *testing.T, request sdktrace.ReadOnlySpan, last string) []sdktrace.ReadOnlySpan {
	t.Helper()
	id := request.SpanContext().TraceID()
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(2 * time.Millisecond) {
		spans := teltest.InTrace(x.rec.Ended(), id)
		for _, s := range spans {
			if s.Name() == last {
				return spans
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("the trace never got a span %q:\n%s", last, teltest.Shape(spans))
		}
	}
}

// ids hides what differs on every run when a tree is logged.
var ids = []string{"kubedojo.session.id", "kubedojo.environment.alias"}

func wantShape(t *testing.T, spans []sdktrace.ReadOnlySpan, want string) {
	t.Helper()
	if got := teltest.Shape(spans); got != strings.TrimLeft(want, "\n") {
		t.Errorf("the trace has the shape\n%s\nwant\n%s", got, strings.TrimLeft(want, "\n"))
	}
}

func str(s sdktrace.ReadOnlySpan, key string) string { return teltest.Attrs(s)[key].AsString() }
func num(s sdktrace.ReadOnlySpan, key string) int64  { return teltest.Attrs(s)[key].AsInt64() }
func flag(s sdktrace.ReadOnlySpan, key string) bool  { return teltest.Attrs(s)[key].AsBool() }

// practice opens a practice session and prepares its first question, each in
// a request of its own, and returns the session and the two request spans.
func (x *traced) practice(t *testing.T) (sid string, open, prepare sdktrace.ReadOnlySpan) {
	t.Helper()
	open = x.request("POST /api/sessions", func(ctx context.Context) {
		ses, err := x.mgr.PracticeContext(ctx, "x", "")
		if err != nil {
			t.Fatal(err)
		}
		sid = ses.ID
	})
	prepare = x.request("POST /api/sessions/{id}/questions/{q}/prepare", func(ctx context.Context) {
		if err := x.mgr.PrepareContext(ctx, sid, "q01", false); err != nil {
			t.Fatal(err)
		}
	})
	x.trace(t, prepare, "question.prepare")
	return sid, open, prepare
}

// Preparing a question reads as: the request, the preparation, the sandbox
// with its three phases, the setup with the command it ran.
func TestTraceOfPreparingAQuestion(t *testing.T) {
	x := newTraced(t, tracedCatalog(t), -1)
	sid, open, prepare := x.practice(t)

	// Opening the session is one span that says whether it was created.
	opened := x.trace(t, open, "session.practice.open")
	wantShape(t, opened, `
POST /api/sessions
   session.practice.open
`)
	if s := x.rec.Named("session.practice.open")[0]; !flag(s, "kubedojo.session.created") || str(s, "kubedojo.session.id") != sid || str(s, "kubedojo.track") != "x" {
		t.Errorf("session.practice.open: %v", s.Attributes())
	}
	x.request("POST /api/sessions", func(ctx context.Context) {
		if again, err := x.mgr.PracticeContext(ctx, "x", ""); err != nil || again.ID != sid {
			t.Fatalf("the second call returned %v, %v", again, err)
		}
	})
	if s := x.rec.Named("session.practice.open")[1]; flag(s, "kubedojo.session.created") || str(s, "kubedojo.session.id") != sid {
		t.Errorf("session.practice.open for the session that exists: %v", s.Attributes())
	}

	spans := x.trace(t, prepare, "question.prepare")
	t.Logf("prepare a practice question:\n%s", teltest.Tree(spans, ids...))
	wantShape(t, spans, `
POST /api/sessions/{id}/questions/{q}/prepare
   question.prepare
      sandbox.provision
         sandbox.host.acquire
         sandbox.env.create
         sandbox.terminal.connect
      question.setup
         sandbox.exec
`)
	for _, s := range spans[:len(spans)-1] {
		if s.Name() != prepare.Name() && (str(s, "kubedojo.session.id") != sid || str(s, "kubedojo.track") != "x" || str(s, "kubedojo.mode") != "practice") {
			t.Errorf("%s lacks the session, the track or the mode: %v", s.Name(), s.Attributes())
		}
	}
	create := x.rec.Named("sandbox.env.create")[0]
	if str(create, "kubedojo.environment") != "base" || str(create, "kubedojo.environment.type") != "cluster" ||
		num(create, "kubedojo.environment.nodes") != 2 || str(create, "kubedojo.environment.alias") == "" {
		t.Errorf("sandbox.env.create: %v", create.Attributes())
	}
	exec := x.rec.Named("sandbox.exec")[0]
	if str(exec, "kubedojo.exec.purpose") != session.ExecSetup || str(exec, "kubedojo.exec.target") != "control-plane" ||
		str(exec, "kubedojo.question") != "q01" || num(exec, "process.exit.code") != 0 {
		t.Errorf("sandbox.exec of the setup: %v", exec.Attributes())
	}

	// Starting over cleans up first: two commands, told apart by purpose.
	again := x.request("POST /api/sessions/{id}/questions/{q}/prepare", func(ctx context.Context) {
		if err := x.mgr.PrepareContext(ctx, sid, "q01", true); err != nil {
			t.Fatal(err)
		}
	})
	spans = x.trace(t, again, "question.prepare")
	wantShape(t, spans, `
POST /api/sessions/{id}/questions/{q}/prepare
   question.prepare
      question.setup
         sandbox.exec
         sandbox.exec
`)
	var purposes []string
	for _, s := range spans {
		if s.Name() == "sandbox.exec" {
			purposes = append(purposes, str(s, "kubedojo.exec.purpose"))
		}
	}
	if strings.Join(purposes, ",") != "cleanup,setup" {
		t.Errorf("purposes of the commands of a reset: %v", purposes)
	}
}

// A setup script that fails is an error of the command, of the setup and of
// the preparation: that is a fault of the content or the server.
func TestTraceOfAFailedSetup(t *testing.T) {
	x := newTraced(t, tracedCatalog(t), -1)
	x.drv.answers(func(_, script string) (sandbox.ExecResult, error) {
		if script == "setup" {
			return sandbox.ExecResult{ExitCode: 7, Stderr: "boom"}, nil
		}
		return sandbox.ExecResult{}, nil
	})
	_, _, prepare := x.practice(t)
	spans := x.trace(t, prepare, "question.prepare")
	for _, name := range []string{"sandbox.exec", "question.setup", "question.prepare"} {
		for _, s := range spans {
			if s.Name() == name && s.Status().Code != codes.Error {
				t.Errorf("%s has status %v after a failed setup", name, s.Status().Code)
			}
		}
	}
	if exec := x.rec.Named("sandbox.exec")[0]; num(exec, "process.exit.code") != 7 || exec.Status().Description != "exit code 7" {
		t.Errorf("sandbox.exec: %v, status %q", exec.Attributes(), exec.Status().Description)
	}
}

// Grading a question reads as: the request, the question with its score, and
// one span per check with its outcome. A wrong answer is no error anywhere.
func TestTraceOfAPracticeCheck(t *testing.T) {
	x := newTraced(t, tracedCatalog(t), -1)
	sid, _, _ := x.practice(t)
	x.drv.answers(func(_, script string) (sandbox.ExecResult, error) {
		if script == "check c2" || script == "check c4" {
			return sandbox.ExecResult{ExitCode: 1, Stderr: "what the learner wrote"}, nil
		}
		return sandbox.ExecResult{}, nil
	})
	var res *checker.Result
	check := x.request("POST /api/sessions/{id}/questions/{q}/check", func(ctx context.Context) {
		var err error
		if res, err = x.mgr.Check(ctx, sid, "q01"); err != nil {
			t.Fatal(err)
		}
	})
	spans := x.trace(t, check, "question.check")
	t.Logf("check a practice question:\n%s", teltest.Tree(spans, ids...))
	wantShape(t, spans, `
POST /api/sessions/{id}/questions/{q}/check
   question.check
      check.run
      check.run
      check.run
      check.run
      check.run
`)
	q := x.rec.Named("question.check")[0]
	if str(q, "kubedojo.question") != "q01" || str(q, "kubedojo.question.type") != "task" || num(q, "kubedojo.question.score") != 3 ||
		num(q, "kubedojo.question.max") != 5 || flag(q, "kubedojo.question.passed") || num(q, "kubedojo.checks.total") != 5 ||
		num(q, "kubedojo.checks.failed") != 2 || int(num(q, "kubedojo.question.score")) != res.Score {
		t.Errorf("question.check: %v", q.Attributes())
	}
	for _, s := range spans {
		if s.Status().Code != codes.Unset {
			t.Errorf("%s has status %v: a question that was not solved is no error", s.Name(), s.Status().Code)
		}
		for _, a := range s.Attributes() {
			if strings.Contains(a.Value.String(), "what the learner wrote") {
				t.Errorf("%s: attribute %s carries the output of a check", s.Name(), a.Key)
			}
		}
	}
	failed := 0
	for _, s := range x.rec.Named("check.run") {
		if str(s, "kubedojo.check.outcome") == checker.OutcomeFailed && !flag(s, "kubedojo.check.passed") {
			failed++
		}
	}
	if failed != 2 {
		t.Errorf("%d check.run spans with the outcome failed, want 2", failed)
	}

	// A check that cannot be evaluated is the server's fault: the check and
	// the question are errors, named grader_error. The API still answers.
	x.drv.answers(func(_, script string) (sandbox.ExecResult, error) {
		if script == "check c3" {
			return sandbox.ExecResult{}, errors.New("cannot reach the sandbox")
		}
		return sandbox.ExecResult{}, nil
	})
	broken := x.request("POST /api/sessions/{id}/questions/{q}/check", func(ctx context.Context) {
		if _, err := x.mgr.Check(ctx, sid, "q01"); err != nil {
			t.Fatal(err)
		}
	})
	spans = x.trace(t, broken, "question.check")
	q = x.rec.Named("question.check")[1]
	if q.Status().Code != codes.Error || str(q, "error.type") != checker.OutcomeGraderError || num(q, "kubedojo.checks.failed") != 1 {
		t.Errorf("question.check with a grader error: status %v, %v", q.Status().Code, q.Attributes())
	}
	errs := 0
	for _, s := range spans {
		if s.Name() == "check.run" && s.Status().Code == codes.Error {
			errs++
			if str(s, "kubedojo.check.id") != "c3" || str(s, "kubedojo.check.outcome") != checker.OutcomeGraderError {
				t.Errorf("the check with the error status: %v", s.Attributes())
			}
		}
	}
	if errs != 1 {
		t.Errorf("%d check.run spans with the error status, want 1", errs)
	}

	// A question that cannot be graded yet: the span exists and says why.
	refused := x.request("POST /api/sessions/{id}/questions/{q}/check", func(ctx context.Context) {
		if _, err := x.mgr.Check(ctx, sid, "q02"); !errors.Is(err, session.ErrNotFound) {
			t.Fatalf("checking a question that was never prepared: %v", err)
		}
	})
	spans = x.trace(t, refused, "question.check")
	if q = spans[0]; q.Name() != "question.check" || q.Status().Code != codes.Unset || str(q, "kubedojo.refused") != "not_found" || str(q, "error.type") != "" {
		t.Errorf("the refused check: %s status %v %v", q.Name(), q.Status().Code, q.Attributes())
	}
}

// Building and grading a full exam: seventeen questions with five checks
// each. The trace of the grading names every check once and stays small.
func TestTraceOfAnExam(t *testing.T) {
	x := newTraced(t, tracedCatalog(t), 300*time.Millisecond)
	// q03 is answered just in time: its first check fails on the first look
	// and passes on the next. q05 is not answered at all.
	var mu sync.Mutex
	looks := 0
	x.drv.answers(func(question, script string) (sandbox.ExecResult, error) {
		switch {
		case question == "q03" && script == "check c1":
			mu.Lock()
			defer mu.Unlock()
			if looks++; looks == 1 {
				return sandbox.ExecResult{ExitCode: 1}, nil
			}
		case question == "q05" && strings.HasPrefix(script, "check"):
			return sandbox.ExecResult{ExitCode: 1}, nil
		}
		return sandbox.ExecResult{}, nil
	})

	var sid string
	create := x.request("POST /api/sessions", func(ctx context.Context) {
		ses, err := x.mgr.NewExamContext(ctx, "x", session.ExamOptions{})
		if err != nil {
			t.Fatal(err)
		}
		sid = ses.ID
	})
	spans := x.trace(t, create, "exam.provision")
	// One environment, then a setup with its command per question.
	if n := len(spans); n != 1+1+4+2*tracedQuestions {
		t.Errorf("the trace of building the exam has %d spans:\n%s", n, teltest.Shape(spans))
	}
	if err := x.mgr.Start(sid); err != nil {
		t.Fatal(err)
	}

	finish := x.request("POST /api/sessions/{id}/finish", func(ctx context.Context) {
		if err := x.mgr.Finish(ctx, sid); err != nil {
			t.Fatal(err)
		}
	})
	spans = x.trace(t, finish, "exam.grade")
	t.Logf("grade an exam (%d spans):\n%s", len(spans), teltest.Tree(spans, ids...))

	// request + exam.grade + settle + a span per question + a span per check.
	if want := 3 + tracedQuestions + tracedQuestions*tracedChecks; len(spans) != want {
		t.Errorf("the trace of grading has %d spans, want %d", len(spans), want)
	}
	if len(spans) > 150 {
		t.Errorf("the trace of grading a full exam has %d spans; it should stay readable (150)", len(spans))
	}
	grade := x.rec.Named("exam.grade")[0]
	if grade.Parent().SpanID() != finish.SpanContext().SpanID() {
		t.Error("exam.grade is not a child of the request")
	}
	// 17 questions of 5 points; q05 has none.
	if num(grade, "kubedojo.exam.questions") != tracedQuestions || num(grade, "kubedojo.exam.score") != 80 || num(grade, "kubedojo.exam.max") != 85 ||
		teltest.Attrs(grade)["kubedojo.exam.percent"].AsFloat64() != 94.1 || !flag(grade, "kubedojo.exam.passed") || grade.Status().Code != codes.Unset {
		t.Errorf("exam.grade: status %v, %v", grade.Status().Code, grade.Attributes())
	}
	questions := x.rec.Children(grade)
	if len(questions) != tracedQuestions+1 || questions[len(questions)-1].Name() != "exam.grade.settle" {
		t.Fatalf("children of exam.grade:\n%s", teltest.Shape(spans))
	}
	for _, q := range questions[:tracedQuestions] {
		checks := x.rec.Children(q)
		if q.Name() != "question.check" || len(checks) != tracedChecks || num(q, "kubedojo.checks.total") != tracedChecks {
			t.Errorf("%s %s has %d children: %v", q.Name(), str(q, "kubedojo.question"), len(checks), q.Attributes())
		}
		// The span of the question shows the first look.
		if id := str(q, "kubedojo.question"); flag(q, "kubedojo.question.passed") != (id != "q03" && id != "q05") {
			t.Errorf("question.check %s: passed=%v", id, flag(q, "kubedojo.question.passed"))
		}
	}
	// The second look is one span for the whole exam, with an event per round.
	settle := questions[tracedQuestions]
	rounds := teltest.Events(settle, "round")
	if len(rounds) != 1 || rounds[0]["n"].AsInt64() != 1 || rounds[0]["questions"].AsInt64() != 2 || rounds[0]["passed"].AsInt64() != 1 ||
		num(settle, "kubedojo.settle.rounds") != 1 || num(settle, "kubedojo.settle.passed") != 1 || len(x.rec.Children(settle)) != 0 {
		t.Errorf("exam.grade.settle: %v, rounds %v", settle.Attributes(), rounds)
	}
	for _, s := range spans {
		if s.Status().Code != codes.Unset {
			t.Errorf("%s has status %v in an exam without a server fault", s.Name(), s.Status().Code)
		}
	}
}

// A command outside a check says what it is for. The purpose of Exec comes
// from its context.
func TestTraceOfCommandsOutsideChecks(t *testing.T) {
	x := newTraced(t, tracedCatalog(t), -1)
	sid, _, _ := x.practice(t)
	before := len(x.rec.Named("sandbox.exec"))
	ctx, root := x.rec.Start(context.Background(), "test")
	if _, err := x.mgr.Exec(ctx, sid, "q01", "", "true"); err != nil {
		t.Fatal(err)
	}
	if _, err := x.mgr.Exec(session.WithExecPurpose(ctx, session.ExecSnapshot), sid, "q01", "terminal", "true"); err != nil {
		t.Fatal(err)
	}
	if _, err := x.mgr.Exec(session.WithExecPurpose(ctx, "whatever a caller invents"), sid, "q01", "", "true"); err != nil {
		t.Fatal(err)
	}
	if _, err := x.mgr.Solve(ctx, sid, "q01", "true"); err != nil {
		t.Fatal(err)
	}
	root.End(nil)
	var got []string
	for _, s := range x.rec.Named("sandbox.exec")[before:] {
		got = append(got, str(s, "kubedojo.exec.purpose")+"@"+str(s, "kubedojo.exec.target"))
		if s.Parent().SpanID() != x.rec.Named("test")[0].SpanContext().SpanID() {
			t.Errorf("sandbox.exec is not a child of the caller's span")
		}
	}
	if want := "diagnostic@control-plane,snapshot@terminal,diagnostic@control-plane,solution@control-plane"; strings.Join(got, ",") != want {
		t.Errorf("purposes and targets: %v, want %s", got, want)
	}
}

// Attaching a terminal is a short span of its own trace. The terminal's end,
// hours later, is a log line in that trace and a sample of the duration.
func TestTraceOfATerminal(t *testing.T) {
	x := newTraced(t, tracedCatalog(t), -1)
	sid, _, _ := x.practice(t)
	// The WebSocket request is not traced: its context has no span.
	tty, err := x.mgr.Attach(context.Background(), sid, 120, 40)
	if err != nil {
		t.Fatal(err)
	}
	attach := x.ended(t, "terminal.attach", 1)[0]
	if attach.Parent().IsValid() || str(attach, "kubedojo.session.id") != sid || num(attach, "terminal.cols") != 120 {
		t.Errorf("terminal.attach: parent %v, %v", attach.Parent().SpanID(), attach.Attributes())
	}
	// The fake terminal echoes through a pipe: a write waits for its read.
	typed := make(chan struct{})
	go func() { _, _ = tty.Write([]byte("ls\n")); close(typed) }()
	buf := make([]byte, 16)
	if n, err := tty.Read(buf); err != nil || n != 3 {
		t.Fatalf("read %d bytes, %v", n, err)
	}
	<-typed
	if err := session.CloseTerminal(tty, session.TerminalClientClosed); err != nil {
		t.Fatal(err)
	}
	_ = tty.Close() // a second close reports nothing

	var closed []map[string]any
	for _, l := range x.logs.lines(t) {
		if l["msg"] == "terminal closed" {
			closed = append(closed, l)
		}
	}
	if len(closed) != 1 {
		t.Fatalf("%d lines say the terminal closed: %v", len(closed), closed)
	}
	l := closed[0]
	if l["session"] != sid || l["reason"] != session.TerminalClientClosed || l["bytes_in"] != float64(3) || l["bytes_out"] != float64(3) ||
		l["trace_id"] != attach.SpanContext().TraceID().String() {
		t.Errorf("the line of the terminal's end: %v", l)
	}
	if _, ok := l["duration_s"].(float64); !ok {
		t.Errorf("duration_s = %v", l["duration_s"])
	}
	x.mu.Lock()
	recorded := len(x.terminals)
	x.mu.Unlock()
	if recorded != 1 {
		t.Errorf("the duration of the terminal was recorded %d times", recorded)
	}

	// The limit is a failure of the attach span, named as such.
	x.mgr.SetMaxTerminals(1)
	first, err := x.mgr.Attach(context.Background(), sid, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if _, err := x.mgr.Attach(context.Background(), sid, 80, 24); !errors.Is(err, session.ErrTerminalLimit) {
		t.Fatalf("attach beyond the limit: %v", err)
	}
	if s := x.ended(t, "terminal.attach", 3)[2]; s.Status().Code != codes.Unset || str(s, "kubedojo.refused") != "limit" {
		t.Errorf("terminal.attach beyond the limit: status %v %v", s.Status().Code, s.Attributes())
	}
}

// The manager hands the store observer to its store.
func TestStoreOperationsAreObserved(t *testing.T) {
	x := newTraced(t, tracedCatalog(t), -1)
	x.practice(t)
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.storeOps["save_session"] == 0 {
		t.Errorf("operations seen: %v", x.storeOps)
	}
	for op := range x.storeOps {
		known := false
		for _, k := range store.Operations {
			known = known || k == op
		}
		if !known {
			t.Errorf("operation %q is not in store.Operations", op)
		}
	}
}

// Choice questions are graded in memory. Grading one is the same span as for
// a task, with the fields that apply; the exam is one span.
func TestTraceOfAQuiz(t *testing.T) {
	cat, err := catalog.Load(quizFS())
	if err != nil {
		t.Fatal(err)
	}
	x := newTraced(t, cat, -1)
	track := cat.Tracks["quiz"]
	q := track.Questions[0]
	ses, err := x.mgr.Practice("quiz", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := x.mgr.Prepare(ses.ID, q.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := x.mgr.Answer(ses.ID, q.ID, q.Choice.Correct()); err != nil {
		t.Fatal(err)
	}
	check := x.request("POST /api/sessions/{id}/questions/{q}/check", func(ctx context.Context) {
		if _, err := x.mgr.Check(ctx, ses.ID, q.ID); err != nil {
			t.Fatal(err)
		}
	})
	spans := x.trace(t, check, "question.check")
	wantShape(t, spans, `
POST /api/sessions/{id}/questions/{q}/check
   question.check
`)
	s := x.rec.Named("question.check")[0]
	if str(s, "kubedojo.question.type") != "choice" || num(s, "kubedojo.question.score") != 1 || !flag(s, "kubedojo.question.passed") {
		t.Errorf("question.check of a choice question: %v", s.Attributes())
	}
	if _, ok := teltest.Attrs(s)["kubedojo.checks.total"]; ok {
		t.Errorf("a choice question has no checks: %v", s.Attributes())
	}
	// Grading it again is refused; the span says so.
	x.request("POST /api/sessions/{id}/questions/{q}/check", func(ctx context.Context) {
		if _, err := x.mgr.Check(ctx, ses.ID, q.ID); !errors.Is(err, session.ErrConflict) {
			t.Fatalf("second check: %v", err)
		}
	})
	if s := x.rec.Named("question.check")[1]; s.Status().Code != codes.Unset || str(s, "kubedojo.refused") != "conflict" {
		t.Errorf("the refused check: status %v, %v", s.Status().Code, s.Attributes())
	}

	exam, err := x.mgr.NewExam("quiz", session.ExamOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := x.mgr.Start(exam.ID); err != nil {
		t.Fatal(err)
	}
	finish := x.request("POST /api/sessions/{id}/finish", func(ctx context.Context) {
		if err := x.mgr.Finish(ctx, exam.ID); err != nil {
			t.Fatal(err)
		}
	})
	spans = x.trace(t, finish, "exam.grade")
	wantShape(t, spans, `
POST /api/sessions/{id}/finish
   exam.grade
`)
	grade := x.rec.Named("exam.grade")[0]
	if num(grade, "kubedojo.exam.questions") == 0 || num(grade, "kubedojo.exam.score") != 0 || flag(grade, "kubedojo.exam.passed") {
		t.Errorf("exam.grade of a quiz nobody answered: %v", grade.Attributes())
	}
}

// Without a tracer in the hooks the manager uses the one of the context, and
// without any it traces nothing.
func TestTracerOfTheContextIsUsedWithoutHooks(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	mgr, err := session.NewManager(session.Config{Catalog: tracedCatalog(t), Store: st, Driver: &sandboxtest.Driver{}})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	rec := teltest.New()
	ctx, root := rec.Start(context.Background(), "request")
	ses, err := mgr.PracticeContext(ctx, "x", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.PrepareContext(ctx, ses.ID, "q01", false); err != nil {
		t.Fatal(err)
	}
	root.End(nil)
	for deadline := time.Now().Add(10 * time.Second); len(rec.Named("question.prepare")) == 0; time.Sleep(2 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("question.prepare was not traced:\n%s", teltest.Shape(rec.Ended()))
		}
	}
	// Without a Detach hook the background work is not in the trace of the
	// request, but it is traced, and below one root.
	prepare := rec.Named("question.prepare")[0]
	if len(rec.Children(prepare)) != 2 {
		t.Errorf("children of question.prepare:\n%s", teltest.Shape(rec.Ended()))
	}
	_ = span.Current(ctx)
}
