package session

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/sandbox"
	"github.com/Mistral-valaise/kubedojo/internal/span"
)

// ErrTerminalLimit is returned by Attach when a session already has as many
// open terminals as allowed.
var ErrTerminalLimit = errors.New("terminal limit reached")

// DefaultMaxTerminals is the number of terminals one session may have open at
// the same time. Each one is a shell (and a Docker exec) in the sandbox.
const DefaultMaxTerminals = 4

// terminals tracks the open terminals of every session, so that they can be
// counted against the limit and closed when the sandbox goes away.
type terminals struct {
	mu   sync.Mutex
	max  int
	open map[string]map[*terminal]struct{}
}

// terminal is an attached shell that gives its slot back when closed.
type terminal struct {
	reg     *terminals
	session string
	// Guarded by reg.mu. tty is nil while the shell is being started and is
	// not changed once the terminal was handed to the caller.
	tty    sandbox.TTY
	closed bool
	// What the terminal did, for the line that is logged when it ends: when
	// the shell was handed over, and the bytes that went through it. ended
	// is called once with the reason, after the shell was closed. opened and
	// ended are set together with tty.
	opened  time.Time
	in, out atomic.Int64
	ended   func(t *terminal, reason string)
}

// Why a terminal ended, as the log line of its end names it.
const (
	TerminalClientClosed  = "client_closed"    // the browser closed the tab or the connection
	TerminalShellExited   = "shell_exited"     // the shell ended, or its container did
	TerminalKeepalive     = "keepalive_failed" // the browser stopped answering pings
	TerminalSessionClosed = "session_closed"   // the sandbox was removed or the server shut down
	terminalClosed        = "closed"           // Close was called without a reason
)

// Read reads what the shell printed: the bytes that go out to the browser.
func (t *terminal) Read(p []byte) (int, error) {
	n, err := t.tty.Read(p)
	t.out.Add(int64(n))
	return n, err
}

// Write writes what the learner typed: the bytes that come in from the browser.
func (t *terminal) Write(p []byte) (int, error) {
	n, err := t.tty.Write(p)
	t.in.Add(int64(n))
	return n, err
}

func (t *terminal) Resize(ctx context.Context, cols, rows uint) error {
	return t.tty.Resize(ctx, cols, rows)
}

// Close ends the shell and frees the slot. It is safe to call more than once
// and from several goroutines (the WebSocket handler and Destroy).
func (t *terminal) Close() error { return t.close(terminalClosed) }

// close is Close with the reason the terminal ended. Only the first call
// counts, and with it its reason.
func (t *terminal) close(reason string) error {
	if reason == "" {
		reason = terminalClosed
	}
	t.reg.mu.Lock()
	if t.closed {
		t.reg.mu.Unlock()
		return nil
	}
	t.closed = true
	tty, ended := t.tty, t.ended
	delete(t.reg.open[t.session], t)
	if len(t.reg.open[t.session]) == 0 {
		delete(t.reg.open, t.session)
	}
	t.reg.mu.Unlock()
	if tty == nil {
		return nil
	}
	err := tty.Close()
	if ended != nil {
		ended(t, reason)
	}
	return err
}

// CloseTerminal closes a terminal that Attach returned and says why it ended:
// one of the Terminal constants. The reason goes into the log line of the
// terminal's end. For any other TTY it is a plain Close.
func CloseTerminal(tty sandbox.TTY, reason string) error {
	if t, ok := tty.(*terminal); ok {
		return t.close(reason)
	}
	return tty.Close()
}

// start hands the started shell to a reserved terminal. It reports false when
// the terminal was closed in the meantime (the session was destroyed while
// the shell was starting); the caller must then close the shell itself.
// ended is told when and why the terminal ends.
func (t *terminal) start(tty sandbox.TTY, ended func(t *terminal, reason string)) bool {
	t.reg.mu.Lock()
	defer t.reg.mu.Unlock()
	if t.closed {
		return false
	}
	t.tty, t.opened, t.ended = tty, time.Now(), ended
	return true
}

func (r *terminals) limit() int {
	if r.max > 0 {
		return r.max
	}
	return DefaultMaxTerminals
}

// reserve takes a slot for a terminal of the session before its shell is
// started, so that concurrent requests cannot exceed the limit together.
func (r *terminals) reserve(session string) (*terminal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.open[session]) >= r.limit() {
		return nil, fmt.Errorf("%w: at most %d terminals per session", ErrTerminalLimit, r.limit())
	}
	if r.open == nil {
		r.open = map[string]map[*terminal]struct{}{}
	}
	if r.open[session] == nil {
		r.open[session] = map[*terminal]struct{}{}
	}
	t := &terminal{reg: r, session: session}
	r.open[session][t] = struct{}{}
	return t, nil
}

func (r *terminals) count(session string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.open[session])
}

// closeAll closes every terminal of a session.
func (r *terminals) closeAll(session string) {
	r.mu.Lock()
	var all []*terminal
	for t := range r.open[session] {
		all = append(all, t)
	}
	r.mu.Unlock()
	for _, t := range all {
		_ = t.close(TerminalSessionClosed)
	}
}

// SetMaxTerminals sets how many terminals one session may have open at the
// same time. Values below 1 select DefaultMaxTerminals.
func (m *Manager) SetMaxTerminals(n int) {
	m.terms.mu.Lock()
	m.terms.max = n
	m.terms.mu.Unlock()
}

// MaxTerminals returns the per-session terminal limit.
func (m *Manager) MaxTerminals() int {
	m.terms.mu.Lock()
	defer m.terms.mu.Unlock()
	return m.terms.limit()
}

// Terminals returns the number of open terminals of a session.
func (m *Manager) Terminals(id string) int { return m.terms.count(id) }

// Attach opens one more interactive shell on the session's terminal machine.
// Every call is an independent shell with its own scrollback; all of them
// share the sandbox. The caller must Close the returned TTY: that ends the
// shell and frees its slot. Destroying the session closes them all.
//
// Attaching is the span terminal.attach, which ends when the shell is there:
// a terminal stays open for hours, and a span that long would never be seen.
// The terminal's end is a log line (how long it was open, the bytes in and
// out, why it ended) and a sample of the duration, both in the trace of that
// span. Close it with CloseTerminal to name the reason.
func (m *Manager) Attach(ctx context.Context, id string, cols, rows uint) (_ sandbox.TTY, err error) {
	s, err := m.get(id)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	n := len(s.Envs)
	m.mu.Unlock()
	if n == 0 {
		return nil, fmt.Errorf("%w: sandbox is not ready yet", ErrConflict)
	}
	ctx, sp := m.span(ctx, "terminal.attach", s, span.Int("terminal.cols", int(cols)), span.Int("terminal.rows", int(rows)))
	defer func() { end(sp, err) }()
	t, err := m.terms.reserve(s.ID)
	if err != nil {
		return nil, err
	}
	host, err := m.existing(ctx, s)
	if err != nil {
		_ = t.Close()
		return nil, err
	}
	tty, err := m.drv.Attach(ctx, host, s.ID, cols, rows)
	if err != nil {
		_ = t.Close()
		return nil, err
	}
	// The end of the terminal is reported long after ctx is gone; what stays
	// of it is its trace.
	traced := context.WithoutCancel(ctx)
	ended := func(t *terminal, reason string) {
		d := time.Since(t.opened)
		m.log.InfoContext(traced, "terminal closed", "session", s.ID, "reason", reason,
			"duration_s", d.Round(time.Millisecond).Seconds(), "bytes_in", t.in.Load(), "bytes_out", t.out.Load())
		if m.hooks.TerminalClosed != nil {
			m.hooks.TerminalClosed(traced, d)
		}
	}
	if !t.start(tty, ended) {
		_ = tty.Close()
		return nil, fmt.Errorf("%w: the sandbox was removed", ErrConflict)
	}
	return t, nil
}
