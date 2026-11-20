package session_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Mistral-valaise/kubedojo/content"
	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/dockerhost"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox/sandboxtest"
	"github.com/Mistral-valaise/kubedojo/internal/session"
	"github.com/Mistral-valaise/kubedojo/internal/store"
)

// slowDriver is a driver whose slow operations can be held: creating an
// environment waits for release, and running a script waits while holdExec
// is set. Both give up when their context ends, like the real driver.
type slowDriver struct {
	*sandboxtest.Driver
	release  chan struct{}
	creating chan struct{}
	holdExec atomic.Bool
}

func (d *slowDriver) CreateEnv(ctx context.Context, h dockerhost.Host, sid, alias string, spec sandbox.EnvSpec, keys sandbox.KeyPair, logf func(string, ...any)) error {
	if d.release != nil {
		select {
		case d.creating <- struct{}{}:
		default:
		}
		select {
		case <-d.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return d.Driver.CreateEnv(ctx, h, sid, alias, spec, keys, logf)
}

func (d *slowDriver) Exec(ctx context.Context, h dockerhost.Host, sid string, t sandbox.Target, user, script string, env map[string]string) (sandbox.ExecResult, error) {
	if d.holdExec.Load() {
		<-ctx.Done()
		return sandbox.ExecResult{}, ctx.Err()
	}
	return d.Driver.Exec(ctx, h, sid, t, user, script, env)
}

type slow struct {
	mgr *session.Manager
	drv *slowDriver
	st  *store.Store
	cat *catalog.Catalog
	db  string
}

func newSlow(t *testing.T, holdCreate bool) *slow {
	t.Helper()
	cat, err := catalog.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	z := &slow{cat: cat, db: filepath.Join(t.TempDir(), "test.db"), drv: &slowDriver{Driver: &sandboxtest.Driver{}}}
	if holdCreate {
		z.drv.release, z.drv.creating = make(chan struct{}), make(chan struct{}, 1)
	}
	if z.st, err = store.Open(z.db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { z.st.Close() })
	// A negative GradeSettle: failed checks are not re-run for 20 seconds.
	if z.mgr, err = session.NewManager(session.Config{Catalog: cat, Store: z.st, Driver: z.drv, GradeSettle: -1}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(z.mgr.Close)
	return z
}

// preparing starts preparing the first CKA question and returns once the
// driver is busy creating its cluster.
func (z *slow) preparing(t *testing.T) (sid, qid string) {
	t.Helper()
	ses, err := z.mgr.Practice("cka", "")
	if err != nil {
		t.Fatal(err)
	}
	qid = z.cat.Tracks["cka"].Questions[0].ID
	if err := z.mgr.Prepare(ses.ID, qid, false); err != nil {
		t.Fatal(err)
	}
	select {
	case <-z.drv.creating:
	case <-time.After(10 * time.Second):
		t.Fatal("the driver was never asked to create the cluster")
	}
	return ses.ID, qid
}

func (z *slow) question(t *testing.T, mgr *session.Manager, sid string) *session.QState {
	t.Helper()
	v, err := mgr.View(sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Questions) != 1 {
		t.Fatalf("questions: %+v", v.Questions)
	}
	return v.Questions[0]
}

// A cluster that is being created when the server is asked to stop is given
// the grace period to finish: the learner finds the question ready afterwards.
func TestShutdownWaitsForRunningWork(t *testing.T) {
	z := newSlow(t, true)
	sid, _ := z.preparing(t)
	if n := z.mgr.Running(); n != 1 {
		t.Fatalf("Running() = %d, want 1", n)
	}

	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		done <- z.mgr.Shutdown(ctx)
	}()
	select {
	case err := <-done:
		t.Fatalf("Shutdown returned (%v) while the cluster was still being created", err)
	case <-time.After(200 * time.Millisecond):
	}
	close(z.drv.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Shutdown did not return after the work had finished")
	}
	if q := z.question(t, z.mgr, sid); q.Status != "ready" {
		t.Fatalf("question is %q (%s), want ready", q.Status, q.Error)
	}
}

// When the grace period ends first, the work is cancelled, the question says
// why it failed, and the next server start finds it in that state.
func TestShutdownCancelsWorkAfterTheGracePeriod(t *testing.T) {
	z := newSlow(t, true)
	sid, qid := z.preparing(t)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := z.mgr.Shutdown(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown = %v, want an error that wraps the deadline", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("Shutdown took %s after a grace period of 100ms", d)
	}
	if q := z.question(t, z.mgr, sid); q.Status != "error" || !strings.Contains(q.Error, "interrupted") {
		t.Fatalf("question is %q with error %q, want an interrupted error", q.Status, q.Error)
	}
	if len(z.drv.Created()) != 0 {
		t.Fatalf("a cluster was created after the cancellation: %v", z.drv.Created())
	}

	// Nothing starts any more.
	if err := z.mgr.Prepare(sid, qid, true); !errors.Is(err, session.ErrShuttingDown) {
		t.Fatalf("Prepare after Shutdown = %v, want ErrShuttingDown", err)
	}
	if _, err := z.mgr.NewExam("cka", session.ExamOptions{}); !errors.Is(err, session.ErrShuttingDown) {
		t.Fatalf("NewExam after Shutdown = %v, want ErrShuttingDown", err)
	}
	if err := z.mgr.DestroyLater(sid); !errors.Is(err, session.ErrShuttingDown) {
		t.Fatalf("DestroyLater after Shutdown = %v, want ErrShuttingDown", err)
	}
	// A second Shutdown and Close are harmless.
	if err := z.mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown: %v", err)
	}

	// The state survives the restart.
	next, err := session.NewManager(session.Config{Catalog: z.cat, Store: z.st, Driver: z.drv})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if q := z.question(t, next, sid); q.Status != "error" || !strings.Contains(q.Error, "interrupted") {
		t.Fatalf("after the restart the question is %q with error %q", q.Status, q.Error)
	}
}

// An exam whose grading is cut off must not be recorded: its checks failed
// because the server stopped, not because the answers were wrong.
func TestInterruptedGradingRecordsNoResult(t *testing.T) {
	z := newSlow(t, false)
	ses, err := z.mgr.NewExam("cka", session.ExamOptions{Questions: 2, Minutes: 10})
	if err != nil {
		t.Fatal(err)
	}
	status := func() string {
		v, err := z.mgr.View(ses.ID)
		if err != nil {
			t.Fatal(err)
		}
		return v.Status
	}
	for deadline := time.Now().Add(10 * time.Second); status() != session.StatusReady; {
		if time.Now().After(deadline) {
			t.Fatalf("the exam never became ready: %s", status())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := z.mgr.Start(ses.ID); err != nil {
		t.Fatal(err)
	}
	z.drv.holdExec.Store(true)
	if err := z.mgr.Finish(context.Background(), ses.ID); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := z.mgr.Shutdown(ctx); err == nil {
		t.Fatal("Shutdown reported no interrupted work although an exam was being graded")
	}
	v, err := z.mgr.View(ses.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != session.StatusError || !strings.Contains(v.Error, "interrupted") || v.Report != nil {
		t.Fatalf("session is %q with error %q and report %+v, want an interrupted error and no report", v.Status, v.Error, v.Report)
	}
	exams, err := z.st.Exams("cka", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(exams) != 0 {
		t.Fatalf("an interrupted exam was recorded: %+v", exams)
	}
	attempts, err := z.st.Attempts("cka", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 0 {
		t.Fatalf("attempts of an interrupted exam were recorded: %+v", attempts)
	}
}

// Destroying in the background is work Shutdown waits for.
func TestDestroyLater(t *testing.T) {
	mgr, drv, sid := readySession(t)
	if err := mgr.DestroyLater(sid); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if len(drv.Destroyed) != 1 || drv.Destroyed[0] != sid {
		t.Fatalf("destroyed %v, want [%s]", drv.Destroyed, sid)
	}
	if err := mgr.DestroyLater("nope"); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("DestroyLater of an unknown session = %v", err)
	}
}
