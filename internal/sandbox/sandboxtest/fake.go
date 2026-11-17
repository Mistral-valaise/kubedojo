// Package sandboxtest provides an in-memory sandbox driver for tests: no
// Docker, no clusters, but the same contract as the real drivers.
package sandboxtest

import (
	"context"
	"io"
	"sync"

	"github.com/Mistral-valaise/kubedojo/internal/dockerhost"
	"github.com/Mistral-valaise/kubedojo/internal/sandbox"
)

// Driver is a sandbox.Driver whose clusters come up instantly and whose
// terminals echo what is typed into them.
type Driver struct {
	mu        sync.Mutex
	ttys      []*TTY
	Destroyed []string
	// AttachErr, when set, makes Attach fail.
	AttachErr error
}

func (d *Driver) Name() string { return "kind" }

func (d *Driver) CreateEnv(context.Context, dockerhost.Host, string, string, sandbox.EnvSpec, sandbox.KeyPair, func(string, ...any)) error {
	return nil
}

func (d *Driver) EnsureTerminal(context.Context, dockerhost.Host, string, []string, sandbox.KeyPair) error {
	return nil
}

func (d *Driver) Exec(context.Context, dockerhost.Host, string, sandbox.Target, string, string, map[string]string) (sandbox.ExecResult, error) {
	return sandbox.ExecResult{}, nil
}

func (d *Driver) Attach(_ context.Context, _ dockerhost.Host, _ string, cols, rows uint) (sandbox.TTY, error) {
	if d.AttachErr != nil {
		return nil, d.AttachErr
	}
	r, w := io.Pipe()
	t := &TTY{r: r, w: w, Cols: cols, Rows: rows}
	d.mu.Lock()
	d.ttys = append(d.ttys, t)
	d.mu.Unlock()
	return t, nil
}

func (d *Driver) DeleteEnv(context.Context, dockerhost.Host, string) error { return nil }

func (d *Driver) Destroy(_ context.Context, _ dockerhost.Host, sessionID string, _ []string) error {
	d.mu.Lock()
	d.Destroyed = append(d.Destroyed, sessionID)
	d.mu.Unlock()
	return nil
}

// TTYs returns every terminal that was ever attached, in order.
func (d *Driver) TTYs() []*TTY {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]*TTY(nil), d.ttys...)
}

// Open returns the number of terminals that are attached and not closed.
func (d *Driver) Open() int {
	n := 0
	for _, t := range d.TTYs() {
		if !t.Closed() {
			n++
		}
	}
	return n
}

// TTY echoes its input, like a terminal in raw mode with a cat running.
type TTY struct {
	r *io.PipeReader
	w *io.PipeWriter

	mu         sync.Mutex
	Cols, Rows uint
	closed     bool
}

func (t *TTY) Read(p []byte) (int, error)  { return t.r.Read(p) }
func (t *TTY) Write(p []byte) (int, error) { return t.w.Write(p) }

func (t *TTY) Close() error {
	t.mu.Lock()
	t.closed = true
	t.mu.Unlock()
	t.w.Close()
	return t.r.Close()
}

func (t *TTY) Resize(_ context.Context, cols, rows uint) error {
	t.mu.Lock()
	t.Cols, t.Rows = cols, rows
	t.mu.Unlock()
	return nil
}

func (t *TTY) Closed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closed
}

// Size returns the last size the terminal was given.
func (t *TTY) Size() (cols, rows uint) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Cols, t.Rows
}
