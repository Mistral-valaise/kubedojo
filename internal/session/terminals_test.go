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
	"github.com/Mistral-valaise/kubedojo/internal/sandbox"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox/sandboxtest"
	"github.com/Mistral-valaise/kubedojo/internal/session"
	"github.com/Mistral-valaise/kubedojo/internal/store"
)

// readySession returns a manager with one practice session whose sandbox is up.
func readySession(t *testing.T) (*session.Manager, *sandboxtest.Driver, string) {
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
	mgr, err := session.NewManager(session.Config{Catalog: cat, Store: st, Driver: drv})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mgr.Close)
	ses, err := mgr.Practice("cka", "")
	if err != nil {
		t.Fatal(err)
	}
	q := cat.Tracks["cka"].Questions[0]
	if err := mgr.Prepare(ses.ID, q.ID, false); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		v, err := mgr.View(ses.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(v.Questions) > 0 && v.Questions[0].Status == "ready" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("question never became ready: %+v", v.Questions)
		}
		time.Sleep(10 * time.Millisecond)
	}
	return mgr, drv, ses.ID
}

func TestAttachBeforeSandboxIsReady(t *testing.T) {
	cat, _ := catalog.Load(content.FS)
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	mgr, err := session.NewManager(session.Config{Catalog: cat, Store: st, Driver: &sandboxtest.Driver{}})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	ses, _ := mgr.Practice("cka", "")
	if _, err := mgr.Attach(context.Background(), ses.ID, 80, 24); !errors.Is(err, session.ErrConflict) {
		t.Fatalf("Attach before any cluster exists = %v, want ErrConflict", err)
	}
	if n := mgr.Terminals(ses.ID); n != 0 {
		t.Errorf("a failed attach holds %d slot(s)", n)
	}
}

func TestTerminalLimitAndRelease(t *testing.T) {
	mgr, drv, id := readySession(t)
	mgr.SetMaxTerminals(2)
	ctx := context.Background()

	a, err := mgr.Attach(ctx, id, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	b, err := mgr.Attach(ctx, id, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if a == b || len(drv.TTYs()) != 2 {
		t.Fatal("two terminals must be two independent shells")
	}
	if _, err := mgr.Attach(ctx, id, 80, 24); !errors.Is(err, session.ErrTerminalLimit) {
		t.Fatalf("third terminal with a limit of 2 = %v, want ErrTerminalLimit", err)
	}
	if len(drv.TTYs()) != 2 {
		t.Error("a rejected terminal still started a shell")
	}

	// Closing a tab ends its shell and frees the slot.
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if !drv.TTYs()[0].Closed() || drv.TTYs()[1].Closed() {
		t.Error("closing one terminal must end exactly its own shell")
	}
	if n := mgr.Terminals(id); n != 1 {
		t.Errorf("%d terminals counted after closing one of two", n)
	}
	a.Close() // closing twice must not free a second slot
	c, err := mgr.Attach(ctx, id, 80, 24)
	if err != nil {
		t.Fatalf("a freed slot cannot be used again: %v", err)
	}
	if _, err := mgr.Attach(ctx, id, 80, 24); !errors.Is(err, session.ErrTerminalLimit) {
		t.Fatalf("closing a terminal twice freed two slots: %v", err)
	}
	b.Close()
	c.Close()
	if mgr.Terminals(id) != 0 || drv.Open() != 0 {
		t.Errorf("left open: %d counted, %d shells", mgr.Terminals(id), drv.Open())
	}
}

func TestTerminalLimitUnderConcurrency(t *testing.T) {
	mgr, drv, id := readySession(t)
	mgr.SetMaxTerminals(3)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var got []sandbox.TTY
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if tty, err := mgr.Attach(context.Background(), id, 80, 24); err == nil {
				mu.Lock()
				got = append(got, tty)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(got) != 3 || drv.Open() != 3 {
		t.Fatalf("20 concurrent requests opened %d terminals (%d shells), want 3", len(got), drv.Open())
	}
	for _, tty := range got {
		tty.Close()
	}
}

func TestFailedAttachFreesSlot(t *testing.T) {
	mgr, drv, id := readySession(t)
	mgr.SetMaxTerminals(1)
	drv.AttachErr = errors.New("docker is down")
	if _, err := mgr.Attach(context.Background(), id, 80, 24); err == nil {
		t.Fatal("want the driver error")
	}
	drv.AttachErr = nil
	tty, err := mgr.Attach(context.Background(), id, 80, 24)
	if err != nil {
		t.Fatalf("a failed attach kept its slot: %v", err)
	}
	tty.Close()
}

func TestDestroyClosesTerminals(t *testing.T) {
	mgr, drv, id := readySession(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := mgr.Attach(ctx, id, 80, 24); err != nil {
			t.Fatal(err)
		}
	}
	if err := mgr.Destroy(ctx, id); err != nil {
		t.Fatal(err)
	}
	if drv.Open() != 0 || mgr.Terminals(id) != 0 {
		t.Errorf("after Destroy: %d shells open, %d counted", drv.Open(), mgr.Terminals(id))
	}
	if len(drv.Destroyed) != 1 {
		t.Errorf("sandbox destroyed %d times", len(drv.Destroyed))
	}
}

func TestDefaultTerminalLimit(t *testing.T) {
	mgr, _, _ := readySession(t)
	if mgr.MaxTerminals() != session.DefaultMaxTerminals {
		t.Errorf("default limit %d, want %d", mgr.MaxTerminals(), session.DefaultMaxTerminals)
	}
	mgr.SetMaxTerminals(0)
	if mgr.MaxTerminals() != session.DefaultMaxTerminals {
		t.Error("a limit of 0 must fall back to the default, not block every terminal")
	}
}
