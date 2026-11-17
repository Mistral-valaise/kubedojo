package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Mistral-valaise/kubedojo/content"
	"github.com/Mistral-valaise/kubedojo/internal/api"
	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox/sandboxtest"
	"github.com/Mistral-valaise/kubedojo/internal/session"
	"github.com/Mistral-valaise/kubedojo/internal/store"
)

type fixture struct {
	mgr     *session.Manager
	drv     *sandboxtest.Driver
	srv     *httptest.Server
	id      string
	gauge   atomic.Int64
	wsBase  string
	cleanup func()
}

func newFixture(t *testing.T, maxTerminals int) *fixture {
	t.Helper()
	cat, err := catalog.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{drv: &sandboxtest.Driver{}}
	f.mgr, err = session.NewManager(session.Config{Catalog: cat, Store: st, Driver: f.drv})
	if err != nil {
		t.Fatal(err)
	}
	f.mgr.SetMaxTerminals(maxTerminals)
	ses, err := f.mgr.Practice("cka", "")
	if err != nil {
		t.Fatal(err)
	}
	f.id = ses.ID
	if err := f.mgr.Prepare(ses.ID, cat.Tracks["cka"].Questions[0].ID, false); err != nil {
		t.Fatal(err)
	}
	f.wait(t, "the question to be ready", func() bool {
		v, _ := f.mgr.View(ses.ID)
		return v != nil && len(v.Questions) > 0 && v.Questions[0].Status == "ready"
	})
	s := &api.Server{Catalog: cat, Sessions: f.mgr, Store: st, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		OnTerminal: func(d int) { f.gauge.Add(int64(d)) }}
	f.srv = httptest.NewServer(s.Handler())
	f.wsBase = "ws" + strings.TrimPrefix(f.srv.URL, "http")
	t.Cleanup(func() {
		f.srv.Close()
		f.mgr.Close()
		st.Close()
	})
	return f
}

func (f *fixture) wait(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (f *fixture) dial(t *testing.T, query string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, f.wsBase+"/api/sessions/"+f.id+"/terminal"+query, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func read(t *testing.T, c *websocket.Conn, n int) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var got []byte
	for len(got) < n {
		_, data, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("read after %q: %v", got, err)
		}
		got = append(got, data...)
	}
	return string(got)
}

func TestConfigReportsTerminalLimit(t *testing.T) {
	f := newFixture(t, 3)
	res, err := http.Get(f.srv.URL + "/api/config")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var cfg struct {
		MaxTerminals int `json:"maxTerminals"`
	}
	if err := json.NewDecoder(res.Body).Decode(&cfg); err != nil || cfg.MaxTerminals != 3 {
		t.Fatalf("maxTerminals = %d (%v), want 3", cfg.MaxTerminals, err)
	}
}

// Two terminals of one session are two shells: what is typed into one does
// not show up in the other, and each has its own size.
func TestTerminalsAreIndependent(t *testing.T) {
	f := newFixture(t, 4)
	ctx := context.Background()
	a := f.dial(t, "?cols=100&rows=30")
	b := f.dial(t, "?cols=80&rows=24")
	f.wait(t, "both shells", func() bool { return f.drv.Open() == 2 })
	ttys := f.drv.TTYs()
	if c, r := ttys[0].Size(); c != 100 || r != 30 {
		t.Errorf("first terminal started at %dx%d, want 100x30", c, r)
	}

	if err := a.Write(ctx, websocket.MessageBinary, []byte("watch pods")); err != nil {
		t.Fatal(err)
	}
	if err := b.Write(ctx, websocket.MessageBinary, []byte("kubectl")); err != nil {
		t.Fatal(err)
	}
	if got := read(t, a, len("watch pods")); got != "watch pods" {
		t.Errorf("terminal a read %q", got)
	}
	if got := read(t, b, len("kubectl")); got != "kubectl" {
		t.Errorf("terminal b read %q", got)
	}

	// A resize reaches only the terminal it was sent on.
	if err := b.Write(ctx, websocket.MessageText, []byte(`{"type":"resize","cols":132,"rows":43}`)); err != nil {
		t.Fatal(err)
	}
	f.wait(t, "the resize", func() bool { c, r := ttys[1].Size(); return c == 132 && r == 43 })
	if c, r := ttys[0].Size(); c != 100 || r != 30 {
		t.Errorf("resizing b changed a to %dx%d", c, r)
	}
	if f.gauge.Load() != 2 {
		t.Errorf("connection gauge = %d, want 2", f.gauge.Load())
	}

	// Closing one tab ends its shell and leaves the other one working.
	a.Close(websocket.StatusNormalClosure, "")
	f.wait(t, "the shell of the closed tab to end", func() bool { return ttys[0].Closed() && f.mgr.Terminals(f.id) == 1 })
	if ttys[1].Closed() {
		t.Fatal("closing one terminal ended the other")
	}
	if err := b.Write(ctx, websocket.MessageBinary, []byte("!")); err != nil {
		t.Fatal(err)
	}
	if got := read(t, b, 1); got != "!" {
		t.Errorf("terminal b read %q after a was closed", got)
	}
	b.Close(websocket.StatusNormalClosure, "")
	f.wait(t, "all shells to end", func() bool { return f.drv.Open() == 0 && f.mgr.Terminals(f.id) == 0 && f.gauge.Load() == 0 })
}

func TestTerminalLimitClosesWithCode(t *testing.T) {
	f := newFixture(t, 1)
	first := f.dial(t, "")
	f.wait(t, "the first shell", func() bool { return f.drv.Open() == 1 })

	second := f.dial(t, "")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, err := second.Read(ctx)
	if websocket.CloseStatus(err) != api.CloseTerminalLimit {
		t.Fatalf("second terminal with a limit of 1: %v, want close code %d", err, api.CloseTerminalLimit)
	}
	if len(f.drv.TTYs()) != 1 {
		t.Error("a rejected terminal started a shell")
	}

	// The slot comes back when the tab is closed — also without a close
	// handshake, as when the browser tab is killed or the network drops.
	first.CloseNow()
	f.wait(t, "the slot to be freed", func() bool { return f.mgr.Terminals(f.id) == 0 && f.drv.Open() == 0 })
	third := f.dial(t, "")
	f.wait(t, "the next shell", func() bool { return f.drv.Open() == 1 })
	third.Close(websocket.StatusNormalClosure, "")
}

// Destroying the sandbox (session ended, idle timeout) ends every terminal
// and closes its WebSocket.
func TestDestroyDisconnectsTerminals(t *testing.T) {
	f := newFixture(t, 4)
	a, b := f.dial(t, ""), f.dial(t, "")
	f.wait(t, "both shells", func() bool { return f.drv.Open() == 2 })
	if err := f.mgr.Destroy(context.Background(), f.id); err != nil {
		t.Fatal(err)
	}
	for _, c := range []*websocket.Conn{a, b} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if _, _, err := c.Read(ctx); err == nil {
			t.Error("a terminal stayed connected to a destroyed sandbox")
		}
		cancel()
	}
	f.wait(t, "cleanup", func() bool { return f.drv.Open() == 0 && f.gauge.Load() == 0 })
}

func TestTerminalOfUnknownSession(t *testing.T) {
	f := newFixture(t, 4)
	res, err := http.Get(f.srv.URL + "/api/sessions/nope/terminal")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Errorf("status %d, want 404", res.StatusCode)
	}
}
