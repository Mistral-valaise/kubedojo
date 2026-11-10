// Package sandbox provisions the disposable lab a candidate works in: one
// terminal container (candidate@terminal) plus one or more Kubernetes clusters
// reachable from it via ssh, exactly like the real exam.
package sandbox

import (
	"context"
	"io"

	"github.com/kubedojo/kubedojo/internal/dockerhost"
)

// EnvSpec describes one cluster a track needs.
type EnvSpec struct {
	Name       string `yaml:"-" json:"name"`
	Workers    int    `yaml:"workers" json:"workers"`
	Desc       string `yaml:"description" json:"description"`
	KindConfig string `yaml:"kindConfig" json:"-"` // optional raw kind config overriding the generated one
}

// Target addresses a machine inside the sandbox.
type Target struct {
	Alias string // cluster alias, e.g. cka3962; empty for the terminal
	Role  string // control-plane | worker | worker2 | terminal
}

// ExecResult is the outcome of a non-interactive command.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// TTY is an interactive shell stream.
type TTY interface {
	io.ReadWriteCloser
	Resize(ctx context.Context, cols, rows uint) error
}

// Driver creates sandboxes on a Docker endpoint. v1 ships the kind driver;
// a VM driver (Lima/KubeVirt) can implement the same contract for tasks that
// need a real kernel boundary (AppArmor, gVisor, Falco eBPF, apt upgrades).
type Driver interface {
	Name() string
	// CreateEnv creates a cluster named alias and prepares its nodes.
	CreateEnv(ctx context.Context, h dockerhost.Host, sessionID, alias string, spec EnvSpec, keys KeyPair, logf func(string, ...any)) error
	// EnsureTerminal (re)creates the terminal container wired to the given aliases.
	EnsureTerminal(ctx context.Context, h dockerhost.Host, sessionID string, aliases []string, keys KeyPair) error
	Exec(ctx context.Context, h dockerhost.Host, sessionID string, t Target, user, script string, env map[string]string) (ExecResult, error)
	Attach(ctx context.Context, h dockerhost.Host, sessionID string, cols, rows uint) (TTY, error)
	// DeleteEnv removes a single cluster and leaves the rest of the sandbox alone.
	DeleteEnv(ctx context.Context, h dockerhost.Host, alias string) error
	Destroy(ctx context.Context, h dockerhost.Host, sessionID string, aliases []string) error
}

// KeyPair is the per-session ssh identity.
type KeyPair struct {
	Private string `json:"private"`
	Public  string `json:"public"`
}
