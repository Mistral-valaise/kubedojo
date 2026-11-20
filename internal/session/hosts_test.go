package session_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Mistral-valaise/kubedojo/content"
	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/dockerhost"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox/sandboxtest"
	"github.com/Mistral-valaise/kubedojo/internal/session"
	"github.com/Mistral-valaise/kubedojo/internal/store"
)

// pods is a host provider like hosted mode's: a sandbox exists only after it
// was acquired, and can disappear.
type pods struct {
	mu         sync.Mutex
	exists     map[string]bool
	acquired   int
	released   []string
	reconciled int
	live       map[string]bool // what the manager called live in the last Reconcile
}

func (p *pods) Acquire(_ context.Context, id string, _ func(string, ...any)) (dockerhost.Host, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.acquired++
	p.exists[id] = true
	return dockerhost.Local(), nil
}

func (p *pods) Lookup(_ context.Context, id string) (dockerhost.Host, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return dockerhost.Local(), p.exists[id], nil
}

func (p *pods) Release(_ context.Context, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.exists, id)
	p.released = append(p.released, id)
	return nil
}

func (p *pods) Reconcile(_ context.Context, live func(string) bool) ([]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reconciled++
	p.live = map[string]bool{}
	var removed []string
	for id := range p.exists {
		if live(id) {
			p.live[id] = true
		} else {
			delete(p.exists, id)
			removed = append(removed, id)
		}
	}
	return removed, nil
}

func (p *pods) evict(id string) {
	p.mu.Lock()
	delete(p.exists, id)
	p.mu.Unlock()
}

func (p *pods) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.acquired
}

func hosted(t *testing.T, hosts *pods) (*session.Manager, *sandboxtest.Driver, *store.Store, string) {
	t.Helper()
	cat, err := catalog.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	drv := &sandboxtest.Driver{}
	mgr, err := session.NewManager(session.Config{Catalog: cat, Store: st, Driver: drv, Hosts: hosts})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mgr.Close)
	ses, err := mgr.Practice("cka", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Prepare(ses.ID, cat.Tracks["cka"].Questions[0].ID, false); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		v, _ := mgr.View(ses.ID)
		if v != nil && len(v.Questions) > 0 && v.Questions[0].Status == "ready" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the question never became ready")
		}
	}
	return mgr, drv, st, ses.ID
}

// The sandbox Pod of a session is gone: evicted, deleted, its node rebooted.
// Ending the session must not create a new Pod just to delete it. That took
// up to eight minutes and a privileged Pod for nothing.
func TestDestroyDoesNotRecreateAMissingSandbox(t *testing.T) {
	hosts := &pods{exists: map[string]bool{}}
	mgr, drv, _, sid := hosted(t, hosts)
	before := hosts.count()
	hosts.evict(sid)

	if err := mgr.Destroy(context.Background(), sid); err != nil {
		t.Fatal(err)
	}
	if hosts.count() != before {
		t.Fatalf("Destroy acquired a sandbox %d time(s) for a session whose sandbox was gone", hosts.count()-before)
	}
	if len(drv.Destroyed) != 0 {
		t.Fatalf("the driver was asked to destroy a sandbox that does not exist: %v", drv.Destroyed)
	}
	// What may be left of it is still released.
	if len(hosts.released) != 1 || hosts.released[0] != sid {
		t.Fatalf("released %v", hosts.released)
	}
	if _, err := mgr.View(sid); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("the session is still there: %v", err)
	}
}

// Working on a session whose sandbox is gone says so. Before, a check ran
// against a fresh, empty Pod and failed with an error nobody could read.
func TestWorkOnAMissingSandboxSaysSo(t *testing.T) {
	hosts := &pods{exists: map[string]bool{}}
	mgr, _, st, sid := hosted(t, hosts)
	before := hosts.count()
	hosts.evict(sid)

	if _, err := mgr.Attach(context.Background(), sid, 80, 24); !errors.Is(err, session.ErrSandboxGone) {
		t.Fatalf("Attach = %v, want ErrSandboxGone", err)
	}
	v, _ := mgr.View(sid)
	if _, err := mgr.Exec(context.Background(), sid, v.Questions[0].ID, "", "true"); !errors.Is(err, session.ErrSandboxGone) {
		t.Fatalf("Exec = %v, want ErrSandboxGone", err)
	}
	// A check must not be graded, and not be recorded as a failed attempt.
	if _, err := mgr.Check(context.Background(), sid, v.Questions[0].ID); !errors.Is(err, session.ErrSandboxGone) {
		t.Fatalf("Check = %v, want ErrSandboxGone", err)
	}
	if attempts, _ := st.Attempts("cka", ""); len(attempts) != 0 {
		t.Fatalf("a check without a sandbox was recorded: %+v", attempts)
	}
	if !errors.Is(session.ErrSandboxGone, session.ErrConflict) {
		t.Fatal("ErrSandboxGone must be a conflict, so that the API answers 409 with its text")
	}
	if hosts.count() != before {
		t.Fatal("a sandbox was created for work that needs an existing one")
	}
}

// At start the manager removes sandboxes that belong to no session it knows.
func TestOrphanedSandboxesAreRemovedAtStart(t *testing.T) {
	hosts := &pods{exists: map[string]bool{"left-behind": true}}
	mgr, _, st, sid := hosted(t, hosts)
	mgr.Close()

	// The next server start: the session is known, the other sandbox is not.
	cat, _ := catalog.Load(content.FS)
	next, err := session.NewManager(session.Config{Catalog: cat, Store: st, Driver: &sandboxtest.Driver{}, Hosts: hosts})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		hosts.mu.Lock()
		done := hosts.reconciled >= 2 // once per manager
		gone, kept := !hosts.exists["left-behind"], hosts.exists[sid]
		hosts.mu.Unlock()
		if done {
			if !gone {
				t.Fatal("the sandbox without a session survived the start")
			}
			if !kept {
				t.Fatal("the sandbox of a live session was removed")
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the manager never reconciled")
		}
	}
}

// An exam whose sandbox disappeared is not graded with zero points.
func TestExamWithoutItsSandboxIsNotGraded(t *testing.T) {
	hosts := &pods{exists: map[string]bool{}}
	cat, _ := catalog.Load(content.FS)
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	mgr, err := session.NewManager(session.Config{Catalog: cat, Store: st, Driver: &sandboxtest.Driver{}, Hosts: hosts, GradeSettle: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	ses, err := mgr.NewExam("cka", session.ExamOptions{Questions: 2, Minutes: 10})
	if err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		if v, _ := mgr.View(ses.ID); v != nil && v.Status == session.StatusReady {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the exam never became ready")
		}
	}
	if err := mgr.Start(ses.ID); err != nil {
		t.Fatal(err)
	}
	hosts.evict(ses.ID)
	if err := mgr.Finish(context.Background(), ses.ID); !errors.Is(err, session.ErrSandboxGone) {
		t.Fatalf("Finish = %v, want ErrSandboxGone", err)
	}
	if v, _ := mgr.View(ses.ID); v.Status != session.StatusRunning || v.Report != nil {
		t.Fatalf("the exam is %q with report %+v; want it untouched", v.Status, v.Report)
	}
	if exams, _ := st.Exams("cka", ""); len(exams) != 0 {
		t.Fatalf("an exam without a sandbox was recorded: %+v", exams)
	}
}
