package session

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Mistral-valaise/kubedojo/internal/sandbox"
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
}

func (t *terminal) Read(p []byte) (int, error)  { return t.tty.Read(p) }
func (t *terminal) Write(p []byte) (int, error) { return t.tty.Write(p) }
func (t *terminal) Resize(ctx context.Context, cols, rows uint) error {
	return t.tty.Resize(ctx, cols, rows)
}

// Close ends the shell and frees the slot. It is safe to call more than once
// and from several goroutines (the WebSocket handler and Destroy).
func (t *terminal) Close() error {
	t.reg.mu.Lock()
	if t.closed {
		t.reg.mu.Unlock()
		return nil
	}
	t.closed = true
	tty := t.tty
	delete(t.reg.open[t.session], t)
	if len(t.reg.open[t.session]) == 0 {
		delete(t.reg.open, t.session)
	}
	t.reg.mu.Unlock()
	if tty != nil {
		return tty.Close()
	}
	return nil
}

// start hands the started shell to a reserved terminal. It reports false when
// the terminal was closed in the meantime (the session was destroyed while
// the shell was starting); the caller must then close the shell itself.
func (t *terminal) start(tty sandbox.TTY) bool {
	t.reg.mu.Lock()
	defer t.reg.mu.Unlock()
	if t.closed {
		return false
	}
	t.tty = tty
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
		_ = t.Close()
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
func (m *Manager) Attach(ctx context.Context, id string, cols, rows uint) (sandbox.TTY, error) {
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
	t, err := m.terms.reserve(s.ID)
	if err != nil {
		return nil, err
	}
	host, err := m.hosts.Acquire(ctx, s.ID, func(string, ...any) {})
	if err != nil {
		_ = t.Close()
		return nil, err
	}
	tty, err := m.drv.Attach(ctx, host, s.ID, cols, rows)
	if err != nil {
		_ = t.Close()
		return nil, err
	}
	if !t.start(tty) {
		_ = tty.Close()
		return nil, fmt.Errorf("%w: the sandbox was removed", ErrConflict)
	}
	return t, nil
}
