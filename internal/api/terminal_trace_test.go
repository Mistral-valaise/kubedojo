package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Mistral-valaise/kubedojo/content"
	"github.com/Mistral-valaise/kubedojo/internal/api"
	"github.com/Mistral-valaise/kubedojo/internal/catalog"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox/sandboxtest"
	"github.com/Mistral-valaise/kubedojo/internal/session"
	"github.com/Mistral-valaise/kubedojo/internal/store"
	"github.com/Mistral-valaise/kubedojo/internal/telemetry/teltest"
)

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

// closed returns the log lines that say a terminal ended.
func (s *syncBuffer) closed(t *testing.T) []map[string]any {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []map[string]any
	for _, raw := range bytes.Split(bytes.TrimSpace(s.b.Bytes()), []byte("\n")) {
		var l map[string]any
		if len(raw) == 0 || json.Unmarshal(raw, &l) != nil || l["msg"] != "terminal closed" {
			continue
		}
		out = append(out, l)
	}
	return out
}

// A terminal is open for as long as the learner works. In a trace it is the
// short span of attaching; its end is one log line that says how long it was
// open, how much went through it and why it ended, and one sample of the
// duration.
func TestTerminalEndIsLoggedWithItsReason(t *testing.T) {
	cat, err := catalog.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	logs := &syncBuffer{}
	rec := teltest.New()
	var mu sync.Mutex
	var durations []time.Duration
	f := &fixture{drv: &sandboxtest.Driver{}}
	f.mgr, err = session.NewManager(session.Config{Catalog: cat, Store: st, Driver: f.drv,
		Logger: slog.New(slog.NewJSONHandler(logs, nil)),
		Hooks: session.Hooks{Tracer: rec, TerminalClosed: func(_ context.Context, d time.Duration) {
			mu.Lock()
			durations = append(durations, d)
			mu.Unlock()
		}}})
	if err != nil {
		t.Fatal(err)
	}
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
	s := &api.Server{Catalog: cat, Sessions: f.mgr, Store: st, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	f.srv = httptest.NewServer(s.Handler())
	f.wsBase = "ws" + strings.TrimPrefix(f.srv.URL, "http")
	t.Cleanup(func() {
		f.srv.Close()
		f.mgr.Close()
		st.Close()
	})

	// The learner types, reads the echo and closes the tab.
	a := f.dial(t, "?cols=100&rows=30")
	if err := a.Write(context.Background(), websocket.MessageBinary, []byte("kubectl get pods")); err != nil {
		t.Fatal(err)
	}
	if got := read(t, a, len("kubectl get pods")); got != "kubectl get pods" {
		t.Fatalf("read %q", got)
	}
	a.Close(websocket.StatusNormalClosure, "")
	f.wait(t, "the line of the first terminal's end", func() bool { return len(logs.closed(t)) == 1 })
	l := logs.closed(t)[0]
	if l["reason"] != session.TerminalClientClosed || l["session"] != ses.ID || l["bytes_in"] != float64(16) || l["bytes_out"] != float64(16) {
		t.Errorf("the line of a terminal the learner closed: %v", l)
	}
	if _, ok := l["duration_s"].(float64); !ok {
		t.Errorf("duration_s = %v", l["duration_s"])
	}

	// The sandbox is removed under an open terminal.
	b := f.dial(t, "")
	f.wait(t, "the second shell", func() bool { return f.drv.Open() == 1 })
	if err := f.mgr.Destroy(context.Background(), ses.ID); err != nil {
		t.Fatal(err)
	}
	f.wait(t, "the line of the second terminal's end", func() bool { return len(logs.closed(t)) == 2 })
	if l := logs.closed(t)[1]; l["reason"] != session.TerminalSessionClosed {
		t.Errorf("the line of a terminal whose sandbox was removed: %v", l)
	}
	b.CloseNow()

	// Each attach is a span of its own trace: the WebSocket request has none.
	attached := rec.Named("terminal.attach")
	if len(attached) != 2 || attached[0].Parent().IsValid() || teltest.Attrs(attached[0])["terminal.cols"].AsInt64() != 100 {
		t.Errorf("terminal.attach spans:\n%s", teltest.Tree(attached))
	}
	mu.Lock()
	defer mu.Unlock()
	if len(durations) != 2 {
		t.Errorf("the duration was recorded %d times, want 2", len(durations))
	}
}
