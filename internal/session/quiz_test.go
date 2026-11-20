package session_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/dockerhost"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox/sandboxtest"
	"github.com/Mistral-valaise/kubedojo/internal/session"
	"github.com/Mistral-valaise/kubedojo/internal/store"
)

const explanation = "EXPLANATION-ONLY-IN-THE-CATALOG"

func single(domain string) string {
	return fmt.Sprintf(`
title: Single
domain: %s
stem: Pick the right one.
options:
  - {text: right, correct: true}
  - {text: wrong one, explain: no}
  - {text: wrong two}
  - {text: wrong three}
explanation: %s
references: [{title: Docs, url: "https://kubernetes.io/docs/"}]
`, domain, explanation)
}

const multiple = `
title: Multiple
domain: b
multiple: true
stem: Pick both right ones.
options:
  - {text: right one, correct: true}
  - {text: right two, correct: true}
  - {text: wrong one}
  - {text: wrong two}
explanation: ` + explanation + `
references: [{title: Docs, url: "https://kubernetes.io/docs/"}]
`

// quizFS is a theory track "quiz" with two domains of two questions each, and
// a hands-on track "lab" to compare against.
func quizFS() fstest.MapFS {
	f := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }
	return fstest.MapFS{
		"quiz/track.yaml": f("title: Quiz\nkind: theory\nexam: {durationMinutes: 10, passPercent: 75, questions: 4}\n" +
			"domains: [{id: a, title: A, weight: 50}, {id: b, title: B, weight: 50}]\n"),
		"quiz/lessons/a.md":             f("# A"),
		"quiz/lessons/b.md":             f("# B"),
		"quiz/questions/a1.yaml":        f(single("a")),
		"quiz/questions/a2.yaml":        f(single("a")),
		"quiz/questions/b1.yaml":        f(single("b")),
		"quiz/questions/b2.yaml":        f(multiple),
		"lab/track.yaml":                f("title: Lab\nkubernetesVersion: v1.37\nexam: {durationMinutes: 10, passPercent: 50, questions: 1}\ndomains: [{id: d, title: D, weight: 100}]\nenvironments: {base: {}}\n"),
		"lab/lessons/d.md":              f("# D"),
		"lab/questions/t/question.yaml": f("title: T\ndomain: d\ntask: do\nhints: [h]\nchecks:\n  - {id: c, title: C, type: exec, run: \"true\", explain: e}\n"),
		"lab/questions/t/solution.sh":   f("true"),
		"lab/questions/t/solution.md":   f("do it"),
	}
}

// watchedDriver counts every call that would touch a sandbox.
type watchedDriver struct {
	sandboxtest.Driver
	calls atomic.Int64
}

func (d *watchedDriver) CreateEnv(ctx context.Context, h dockerhost.Host, sid, alias string, spec sandbox.EnvSpec, keys sandbox.KeyPair, logf func(string, ...any)) error {
	d.calls.Add(1)
	return d.Driver.CreateEnv(ctx, h, sid, alias, spec, keys, logf)
}

func (d *watchedDriver) EnsureTerminal(ctx context.Context, h dockerhost.Host, sid string, envs []sandbox.Env, keys sandbox.KeyPair) error {
	d.calls.Add(1)
	return d.Driver.EnsureTerminal(ctx, h, sid, envs, keys)
}

func (d *watchedDriver) Exec(ctx context.Context, h dockerhost.Host, sid string, t sandbox.Target, user, script string, env map[string]string) (sandbox.ExecResult, error) {
	d.calls.Add(1)
	return d.Driver.Exec(ctx, h, sid, t, user, script, env)
}

func (d *watchedDriver) Attach(ctx context.Context, h dockerhost.Host, sid string, cols, rows uint) (sandbox.TTY, error) {
	d.calls.Add(1)
	return d.Driver.Attach(ctx, h, sid, cols, rows)
}

func (d *watchedDriver) DeleteEnv(ctx context.Context, h dockerhost.Host, env sandbox.Env) error {
	d.calls.Add(1)
	return d.Driver.DeleteEnv(ctx, h, env)
}

func (d *watchedDriver) Destroy(ctx context.Context, h dockerhost.Host, sid string, envs []sandbox.Env) error {
	d.calls.Add(1)
	return d.Driver.Destroy(ctx, h, sid, envs)
}

// watchedHosts counts acquisitions: in hosted mode each one may create a pod.
type watchedHosts struct{ acquired, released atomic.Int64 }

func (h *watchedHosts) Acquire(context.Context, string, func(string, ...any)) (dockerhost.Host, error) {
	h.acquired.Add(1)
	return dockerhost.Local(), nil
}

func (h *watchedHosts) Lookup(context.Context, string) (dockerhost.Host, bool, error) {
	return dockerhost.Local(), true, nil
}

func (h *watchedHosts) Release(context.Context, string) error {
	h.released.Add(1)
	return nil
}

type quiz struct {
	mgr   *session.Manager
	cat   *catalog.Catalog
	st    *store.Store
	drv   *watchedDriver
	hosts *watchedHosts
	track *catalog.Track
}

func newQuiz(t *testing.T, maxSessions int) *quiz {
	t.Helper()
	cat, err := catalog.Load(quizFS())
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	z := &quiz{cat: cat, st: st, drv: &watchedDriver{}, hosts: &watchedHosts{}, track: cat.Tracks["quiz"]}
	z.mgr = z.manager(t, maxSessions)
	return z
}

// manager starts a manager on the quiz's store, as a restarted server would.
func (z *quiz) manager(t *testing.T, maxSessions int) *session.Manager {
	t.Helper()
	mgr, err := session.NewManager(session.Config{Catalog: z.cat, Store: z.st, Driver: z.drv, Hosts: z.hosts, MaxSessions: maxSessions})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mgr.Close)
	return mgr
}

func (z *quiz) correct(qid string) []string { return z.track.Question(qid).Choice.Correct() }

func (z *quiz) wrong(qid string) []string {
	for _, o := range z.track.Question(qid).Choice.Options {
		if !o.Correct {
			return []string{o.ID}
		}
	}
	return nil
}

func (z *quiz) state(t *testing.T, sid, qid string) (*session.Session, *session.QState) {
	t.Helper()
	v, err := z.mgr.View(sid)
	if err != nil {
		t.Fatal(err)
	}
	for _, qs := range v.Questions {
		if qs.ID == qid {
			return v, qs
		}
	}
	return v, nil
}

func (z *quiz) attempts(t *testing.T) []store.Attempt {
	t.Helper()
	as, err := z.st.Attempts("quiz", "")
	if err != nil {
		t.Fatal(err)
	}
	return as
}

func sorted(ids []string) []string {
	out := slices.Clone(ids)
	slices.Sort(out)
	return out
}

func TestQuizPracticeFlow(t *testing.T) {
	z := newQuiz(t, 3)
	ctx := context.Background()
	ses, err := z.mgr.Practice("quiz", "")
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := z.mgr.Practice("quiz", ""); again == nil || again.ID != ses.ID {
		t.Fatal("a second call must return the same practice session")
	}

	// Prepare is synchronous: the question is ready when it returns.
	if err := z.mgr.Prepare(ses.ID, "a1", false); err != nil {
		t.Fatal(err)
	}
	v, qs := z.state(t, ses.ID, "a1")
	if qs == nil || qs.Status != "ready" {
		t.Fatalf("after Prepare: %+v", qs)
	}
	var all []string
	for _, o := range z.track.Question("a1").Choice.Options {
		all = append(all, o.ID)
	}
	if !slices.Equal(sorted(qs.Order), sorted(all)) {
		t.Fatalf("order %v is not a permutation of the options %v", qs.Order, all)
	}
	if len(qs.Answer) != 0 || v.Revealed(qs) {
		t.Fatalf("a fresh question has an answer or is revealed: %+v", qs)
	}
	// Preparing again without a reset keeps the order the learner saw.
	_ = z.mgr.Prepare(ses.ID, "a1", false)
	if _, again := z.state(t, ses.ID, "a1"); !slices.Equal(again.Order, qs.Order) {
		t.Errorf("order changed without a reset: %v -> %v", qs.Order, again.Order)
	}

	// A wrong answer: no point, and the question is revealed.
	if err := z.mgr.Answer(ses.ID, "a1", z.wrong("a1")); err != nil {
		t.Fatal(err)
	}
	if v, qs := z.state(t, ses.ID, "a1"); v.Revealed(qs) {
		t.Fatal("answering alone must not reveal")
	}
	res, err := z.mgr.Check(ctx, ses.ID, "a1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed || res.Score != 0 || res.Max != 1 || len(res.Checks) != 0 || res.Checks == nil {
		t.Fatalf("wrong answer graded %+v, want 0/1 without checks", res)
	}
	v, qs = z.state(t, ses.ID, "a1")
	if !v.Revealed(qs) || qs.Last == nil {
		t.Fatal("a graded question must be revealed")
	}
	if err := z.mgr.Answer(ses.ID, "a1", z.correct("a1")); !errors.Is(err, session.ErrConflict) {
		t.Errorf("Answer after the reveal = %v, want ErrConflict", err)
	}
	if _, err := z.mgr.Check(ctx, ses.ID, "a1"); !errors.Is(err, session.ErrConflict) {
		t.Errorf("Check of a graded question = %v, want ErrConflict", err)
	}
	if as := z.attempts(t); len(as) != 1 || as[0].Score != 0 || as[0].Max != 1 || as[0].Mode != session.ModePractice || as[0].Domain != "a" {
		t.Fatalf("attempts = %+v, want one practice attempt with 0/1", as)
	}

	// Reset clears answer and result and draws a new order.
	reshuffled := false
	for i := 0; i < 50 && !reshuffled; i++ {
		if err := z.mgr.Prepare(ses.ID, "a1", true); err != nil {
			t.Fatal(err)
		}
		v, after := z.state(t, ses.ID, "a1")
		if len(after.Answer) != 0 || after.Last != nil || v.Revealed(after) || after.Status != "ready" {
			t.Fatalf("after reset: %+v", after)
		}
		if !slices.Equal(sorted(after.Order), sorted(all)) {
			t.Fatalf("order after reset %v is not a permutation", after.Order)
		}
		reshuffled = !slices.Equal(after.Order, qs.Order)
	}
	if !reshuffled {
		t.Error("50 resets never changed the order of the options")
	}

	// The correct answer scores the point and is recorded as a second attempt.
	if err := z.mgr.Answer(ses.ID, "a1", z.correct("a1")); err != nil {
		t.Fatal(err)
	}
	if res, err = z.mgr.Check(ctx, ses.ID, "a1"); err != nil || !res.Passed || res.Score != 1 {
		t.Fatalf("correct answer graded %+v, %v", res, err)
	}
	if as := z.attempts(t); len(as) != 2 || as[1].Score != 1 {
		t.Fatalf("attempts = %+v, want a second one with 1/1", as)
	}

	// Opening the solution reveals too, and a reset takes that back.
	_ = z.mgr.Prepare(ses.ID, "a1", true)
	if err := z.mgr.Reveal(ses.ID, "a1", 0); err != nil {
		t.Fatal(err)
	}
	if v, qs := z.state(t, ses.ID, "a1"); !v.Revealed(qs) {
		t.Error("an opened solution must reveal the question")
	}
	if err := z.mgr.Answer(ses.ID, "a1", z.correct("a1")); !errors.Is(err, session.ErrConflict) {
		t.Errorf("Answer after the solution was opened = %v, want ErrConflict", err)
	}
	_ = z.mgr.Prepare(ses.ID, "a1", true)
	if err := z.mgr.Answer(ses.ID, "a1", z.correct("a1")); err != nil {
		t.Errorf("Answer after a reset = %v", err)
	}

	// Check without Prepare works: there is nothing to wait for.
	if res, err := z.mgr.Check(ctx, ses.ID, "b1"); err != nil || res.Score != 0 {
		t.Errorf("Check of an untouched question = %+v, %v; want 0 points", res, err)
	}
}

func TestQuizAnswerValidation(t *testing.T) {
	z := newQuiz(t, 3)
	practice, _ := z.mgr.Practice("quiz", "")
	ready, _ := z.mgr.NewExam("quiz", session.ExamOptions{})
	running, _ := z.mgr.NewExam("quiz", session.ExamOptions{})
	finished, _ := z.mgr.NewExam("quiz", session.ExamOptions{})
	for _, id := range []string{running.ID, finished.ID} {
		if err := z.mgr.Start(id); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.mgr.Finish(context.Background(), finished.ID); err != nil {
		t.Fatal(err)
	}
	lab, _ := z.mgr.Practice("lab", "")
	two := z.correct("b2")
	for name, c := range map[string]struct {
		session, question string
		answer            []string
		want              error
	}{
		"one option":                        {practice.ID, "a1", z.correct("a1"), nil},
		"no option clears the answer":       {practice.ID, "a1", nil, nil},
		"unknown option":                    {practice.ID, "a1", []string{"nope00"}, session.ErrInvalid},
		"option of another question":        {practice.ID, "a1", two[:1], session.ErrInvalid},
		"two options for single choice":     {practice.ID, "a1", append(z.correct("a1"), z.wrong("a1")...), session.ErrInvalid},
		"the same option twice":             {practice.ID, "b2", []string{two[0], two[0]}, session.ErrInvalid},
		"two options for a multiple choice": {practice.ID, "b2", two, nil},
		"unknown question":                  {practice.ID, "nope", nil, session.ErrNotFound},
		"unknown session":                   {"nope", "a1", nil, session.ErrNotFound},
		"a task is not answered":            {lab.ID, "t", nil, session.ErrInvalid},
		"exam not started":                  {ready.ID, "a1", z.correct("a1"), session.ErrConflict},
		"exam running":                      {running.ID, "a1", z.correct("a1"), nil},
		"exam running, unknown option":      {running.ID, "a1", []string{"nope00"}, session.ErrInvalid},
		"exam finished":                     {finished.ID, "a1", z.correct("a1"), session.ErrConflict},
	} {
		if err := z.mgr.Answer(c.session, c.question, c.answer); !errors.Is(err, c.want) {
			t.Errorf("%s: Answer = %v, want %v", name, err, c.want)
		}
	}
}

func TestQuizExamFlow(t *testing.T) {
	for name, c := range map[string]struct {
		right   []string // questions answered correctly; the others get a wrong option
		blank   []string // questions left unanswered
		score   int
		percent float64
		passed  bool
		a, b    int // score per domain
	}{
		"all correct":                {right: []string{"a1", "a2", "b1", "b2"}, score: 4, percent: 100, passed: true, a: 2, b: 2},
		"exactly the pass mark":      {right: []string{"a1", "a2", "b1"}, score: 3, percent: 75, passed: true, a: 2, b: 1},
		"below the pass mark":        {right: []string{"a1", "b2"}, score: 2, percent: 50, a: 1, b: 1},
		"unanswered counts as wrong": {right: []string{"a1"}, blank: []string{"a2", "b1", "b2"}, score: 1, percent: 25, a: 1},
	} {
		t.Run(name, func(t *testing.T) {
			z := newQuiz(t, 3)
			ctx := context.Background()
			ses, err := z.mgr.NewExam("quiz", session.ExamOptions{})
			if err != nil {
				t.Fatal(err)
			}
			// Nothing is provisioned: the exam can be started at once.
			v, _ := z.state(t, ses.ID, "")
			if v.Status != session.StatusReady || len(v.Questions) != 4 {
				t.Fatalf("new exam: status %s with %d questions", v.Status, len(v.Questions))
			}
			for _, qs := range v.Questions {
				if qs.Status != "ready" || len(qs.Order) != 4 {
					t.Fatalf("question %s: %+v", qs.ID, qs)
				}
			}
			if err := z.mgr.Start(ses.ID); err != nil {
				t.Fatal(err)
			}
			for _, q := range z.track.Questions {
				switch {
				case slices.Contains(c.blank, q.ID):
					continue
				case slices.Contains(c.right, q.ID):
					err = z.mgr.Answer(ses.ID, q.ID, z.correct(q.ID))
				default:
					// Saved, changed, saved again: the last answer counts.
					if err = z.mgr.Answer(ses.ID, q.ID, z.correct(q.ID)); err == nil {
						err = z.mgr.Answer(ses.ID, q.ID, z.wrong(q.ID))
					}
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			// While the exam runs nothing is graded and nothing is revealed.
			if _, err := z.mgr.Check(ctx, ses.ID, "a1"); !errors.Is(err, session.ErrConflict) {
				t.Errorf("Check during the exam = %v, want ErrConflict", err)
			}
			if err := z.mgr.Reveal(ses.ID, "a1", 0); !errors.Is(err, session.ErrConflict) {
				t.Errorf("Reveal during the exam = %v, want ErrConflict", err)
			}
			v, _ = z.state(t, ses.ID, "")
			for _, qs := range v.Questions {
				if qs.Last != nil || v.Revealed(qs) {
					t.Errorf("question %s is graded or revealed during the exam", qs.ID)
				}
			}
			if v.Report != nil || len(z.attempts(t)) != 0 {
				t.Error("a running exam has a report or attempts")
			}

			// Finish grades inline. The default settle time is 20s: it must not apply.
			start := time.Now()
			if err := z.mgr.Finish(ctx, ses.ID); err != nil {
				t.Fatal(err)
			}
			v, _ = z.state(t, ses.ID, "")
			if took := time.Since(start); took > 2*time.Second {
				t.Errorf("Finish took %s", took)
			}
			if v.Status != session.StatusFinished || v.Report == nil {
				t.Fatalf("after Finish returned: status %s, report %v", v.Status, v.Report)
			}
			r := v.Report
			if r.Score != c.score || r.Max != 4 || r.Percent != c.percent || r.Passed != c.passed || r.PassPercent != 75 {
				t.Errorf("report = %+v, want %d/4 (%.0f%%) passed=%v", r, c.score, c.percent, c.passed)
			}
			got := map[string]int{}
			for _, d := range r.Domains {
				got[d.ID] = d.Score
				if d.Max != 2 {
					t.Errorf("domain %s: max %d, want 2", d.ID, d.Max)
				}
			}
			if got["a"] != c.a || got["b"] != c.b {
				t.Errorf("domain scores = %v, want a=%d b=%d", got, c.a, c.b)
			}
			for _, qs := range v.Questions {
				want := slices.Contains(c.right, qs.ID)
				if qs.Last == nil || qs.Last.Passed != want || !v.Revealed(qs) {
					t.Errorf("question %s: %+v, want passed=%v and revealed", qs.ID, qs.Last, want)
				}
			}
			as := z.attempts(t)
			if len(as) != 4 {
				t.Fatalf("%d attempts recorded, want 4", len(as))
			}
			for _, a := range as {
				if a.Mode != session.ModeExam || a.Max != 1 {
					t.Errorf("attempt %+v", a)
				}
			}
			if ex, _ := z.st.Exams("quiz", ""); len(ex) != 1 || ex[0].Score != c.score || ex[0].Passed != c.passed {
				t.Errorf("exams = %+v", ex)
			}
			// A finished exam stays as it is.
			if err := z.mgr.Finish(ctx, ses.ID); !errors.Is(err, session.ErrConflict) {
				t.Errorf("second Finish = %v, want ErrConflict", err)
			}
			if res, err := z.mgr.Check(ctx, ses.ID, "a1"); err != nil || res.Passed != slices.Contains(c.right, "a1") {
				t.Errorf("Check after the exam = %+v, %v", res, err)
			}
			if n := len(z.attempts(t)); n != 4 {
				t.Errorf("looking at a result recorded an attempt: %d", n)
			}
		})
	}
}

func TestQuizNeverTouchesTheSandbox(t *testing.T) {
	z := newQuiz(t, 3)
	ctx := context.Background()

	practice, err := z.mgr.Practice("quiz", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = z.mgr.Prepare(practice.ID, "a1", false)
	_ = z.mgr.Answer(practice.ID, "a1", z.correct("a1"))
	if _, err := z.mgr.Check(ctx, practice.ID, "a1"); err != nil {
		t.Fatal(err)
	}
	_ = z.mgr.Prepare(practice.ID, "a1", true)
	_ = z.mgr.Reveal(practice.ID, "a1", 0)
	if _, err := z.mgr.Exec(ctx, practice.ID, "a1", "control-plane", "true"); !errors.Is(err, session.ErrConflict) {
		t.Errorf("Exec for a choice question = %v, want ErrConflict", err)
	}
	if _, err := z.mgr.Attach(ctx, practice.ID, 80, 24); !errors.Is(err, session.ErrConflict) {
		t.Errorf("Attach to a session without sandbox = %v, want ErrConflict", err)
	}

	exam, err := z.mgr.NewExam("quiz", session.ExamOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_ = z.mgr.Start(exam.ID)
	_ = z.mgr.Answer(exam.ID, "a1", z.correct("a1"))
	if err := z.mgr.Finish(ctx, exam.ID); err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{practice.ID, exam.ID} {
		if err := z.mgr.Destroy(ctx, id); err != nil {
			t.Fatal(err)
		}
		if _, err := z.mgr.View(id); !errors.Is(err, session.ErrNotFound) {
			t.Errorf("View of a destroyed session = %v, want ErrNotFound", err)
		}
	}
	if n := z.drv.calls.Load(); n != 0 {
		t.Errorf("%d driver calls for a track without sandbox", n)
	}
	if a, r := z.hosts.acquired.Load(), z.hosts.released.Load(); a != 0 || r != 0 {
		t.Errorf("%d host acquisitions and %d releases for a track without sandbox", a, r)
	}
}

func TestQuizSessionsAreNotLimited(t *testing.T) {
	z := newQuiz(t, 1)
	if _, err := z.mgr.Practice("quiz", ""); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := z.mgr.NewExam("quiz", session.ExamOptions{}); err != nil {
			t.Fatalf("quiz exam %d: %v", i+1, err)
		}
	}
	// The one slot is still free for a session that needs a sandbox ...
	if _, err := z.mgr.Practice("lab", ""); err != nil {
		t.Fatalf("the quiz sessions used up the limit: %v", err)
	}
	// ... which does count,
	if _, err := z.mgr.NewExam("lab", session.ExamOptions{}); !errors.Is(err, session.ErrLimit) {
		t.Fatalf("second sandbox session = %v, want ErrLimit", err)
	}
	// and at the limit, quiz sessions can still be started.
	if _, err := z.mgr.NewExam("quiz", session.ExamOptions{}); err != nil {
		t.Fatalf("quiz exam at the limit: %v", err)
	}
	if _, err := z.mgr.Practice("quiz", "someone-else"); err != nil {
		t.Fatalf("quiz practice at the limit: %v", err)
	}
}

func TestQuizAnswersSurviveARestart(t *testing.T) {
	z := newQuiz(t, 3)
	exam, err := z.mgr.NewExam("quiz", session.ExamOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_ = z.mgr.Start(exam.ID)
	for _, q := range []string{"a1", "b2"} {
		if err := z.mgr.Answer(exam.ID, q, z.correct(q)); err != nil {
			t.Fatal(err)
		}
	}
	practice, _ := z.mgr.Practice("quiz", "")
	_ = z.mgr.Answer(practice.ID, "a2", z.wrong("a2"))
	before, _ := z.mgr.View(exam.ID)

	// What is stored is the learner's side only: no answer key, no explanation.
	raws, err := z.st.LoadSessions()
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range raws {
		for _, secret := range []string{explanation, `"correct"`, `"explain`, "right"} {
			if strings.Contains(string(raw), secret) {
				t.Errorf("the stored session contains %q: %s", secret, raw)
			}
		}
	}

	z.mgr = z.manager(t, 3)
	after, err := z.mgr.View(exam.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != session.StatusRunning {
		t.Fatalf("the exam is %s after the restart, want running", after.Status)
	}
	for i, qs := range after.Questions {
		was := before.Questions[i]
		if qs.ID != was.ID || !slices.Equal(qs.Order, was.Order) || !slices.Equal(qs.Answer, was.Answer) {
			t.Errorf("question %s changed over the restart: %+v -> %+v", was.ID, was, qs)
		}
	}
	if _, qs := z.state(t, practice.ID, "a2"); qs == nil || !slices.Equal(qs.Answer, z.wrong("a2")) {
		t.Errorf("the practice answer was lost: %+v", qs)
	}
	if err := z.mgr.Finish(context.Background(), exam.ID); err != nil {
		t.Fatal(err)
	}
	if v, _ := z.mgr.View(exam.ID); v.Report == nil || v.Report.Score != 2 {
		t.Errorf("report after the restart = %+v, want 2 points", v.Report)
	}
}
