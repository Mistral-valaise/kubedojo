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

// ExecCall is one script the driver was asked to run.
type ExecCall struct {
	Session string
	Target  sandbox.Target
	User    string
	Script  string
	Env     map[string]string
}

// Driver is a sandbox.Driver whose environments come up instantly and whose
// terminals echo what is typed into them. It records what it was asked to do.
type Driver struct {
	mu        sync.Mutex
	ttys      []*TTY
	created   []sandbox.Env
	deleted   []sandbox.Env
	wired     [][]sandbox.Env
	execs     []ExecCall
	Destroyed []string
	// DestroyedEnvs holds the environments passed to each Destroy call.
	DestroyedEnvs [][]sandbox.Env
	// AttachErr, when set, makes Attach fail.
	AttachErr error
}

func (d *Driver) Name() string { return "kind" }

func (d *Driver) CreateEnv(_ context.Context, _ dockerhost.Host, _, alias string, spec sandbox.EnvSpec, _ sandbox.KeyPair, _ func(string, ...any)) error {
	d.mu.Lock()
	d.created = append(d.created, sandbox.Env{Alias: alias, Type: spec.Type})
	d.mu.Unlock()
	return nil
}

func (d *Driver) EnsureTerminal(_ context.Context, _ dockerhost.Host, _ string, envs []sandbox.Env, _ sandbox.KeyPair) error {
	d.mu.Lock()
	d.wired = append(d.wired, append([]sandbox.Env(nil), envs...))
	d.mu.Unlock()
	return nil
}

func (d *Driver) Exec(_ context.Context, _ dockerhost.Host, sessionID string, t sandbox.Target, user, script string, env map[string]string) (sandbox.ExecResult, error) {
	d.mu.Lock()
	d.execs = append(d.execs, ExecCall{Session: sessionID, Target: t, User: user, Script: script, Env: env})
	d.mu.Unlock()
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

func (d *Driver) DeleteEnv(_ context.Context, _ dockerhost.Host, env sandbox.Env) error {
	d.mu.Lock()
	d.deleted = append(d.deleted, env)
	d.mu.Unlock()
	return nil
}

func (d *Driver) Destroy(_ context.Context, _ dockerhost.Host, sessionID string, envs []sandbox.Env) error {
	d.mu.Lock()
	d.Destroyed = append(d.Destroyed, sessionID)
	d.DestroyedEnvs = append(d.DestroyedEnvs, append([]sandbox.Env(nil), envs...))
	d.mu.Unlock()
	return nil
}

// Created returns every environment that was created, in order.
func (d *Driver) Created() []sandbox.Env {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]sandbox.Env(nil), d.created...)
}

// Deleted returns every environment that was deleted on its own, in order.
func (d *Driver) Deleted() []sandbox.Env {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]sandbox.Env(nil), d.deleted...)
}

// Wired returns the environments of the most recent EnsureTerminal call.
func (d *Driver) Wired() []sandbox.Env {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.wired) == 0 {
		return nil
	}
	return d.wired[len(d.wired)-1]
}

// Execs returns every script that was run, in order.
func (d *Driver) Execs() []ExecCall {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]ExecCall(nil), d.execs...)
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
