// Package sandbox provisions the disposable lab a candidate works in: one
// terminal container (candidate@terminal) plus the environments of the track,
// reachable from it via ssh, exactly like the real exam. An environment is a
// Kubernetes cluster or, for tracks that teach the shell, one Linux container.
package sandbox

import (
	"context"
	"io"

	"github.com/Mistral-valaise/kubedojo/internal/dockerhost"
)

// Environment types.
const (
	EnvCluster   = "cluster"   // a kind cluster: control plane plus workers
	EnvContainer = "container" // one plain Linux container, no Kubernetes
)

// EnvSpec describes one environment a track needs: a cluster, or a single
// Linux host for tracks that teach the shell.
type EnvSpec struct {
	Name       string `yaml:"-" json:"name"`
	Type       string `yaml:"type" json:"type"` // cluster (default) | container
	Workers    int    `yaml:"workers" json:"workers"`
	Desc       string `yaml:"description" json:"description"`
	KindConfig string `yaml:"kindConfig" json:"-"` // optional raw kind config overriding the generated one
}

// Roles a script or check can run on.
const (
	RoleControlPlane = "control-plane"
	RoleHost         = "host"
	RoleTerminal     = "terminal"
)

// DefaultRole is the machine setup, solution and checks run on unless a check
// says otherwise.
func (e EnvSpec) DefaultRole() string {
	if e.Type == EnvContainer {
		return RoleHost
	}
	return RoleControlPlane
}

// Roles lists the machines of the environment a check may name with `on`.
func (e EnvSpec) Roles() []string {
	if e.Type == EnvContainer {
		return []string{RoleHost, RoleTerminal}
	}
	return []string{RoleControlPlane, "worker", "worker2", RoleTerminal}
}

// Accounts scripts run as.
const (
	UserRoot      = "root"
	UserCandidate = "candidate"
)

// ScriptUser is the account setup, cleanup and checks run as on a role.
func (e EnvSpec) ScriptUser(role string) string {
	if role == RoleTerminal {
		return UserCandidate
	}
	return UserRoot
}

// SolveUser is the account a reference solution runs as on the default role.
// On a Linux host that is the learner's own account, so that the files it
// leaves behind are owned as a learner's would be; on a cluster node ownership
// is not what is graded, and solutions have always run as root there.
func (e EnvSpec) SolveUser() string {
	if e.Type == EnvContainer {
		return UserCandidate
	}
	return UserRoot
}

// Env is one created environment of a session.
type Env struct {
	Alias string // name of the cluster or container, and the learner's ssh name for it
	Type  string // EnvCluster | EnvContainer
}

// Target addresses a machine inside the sandbox.
type Target struct {
	Alias string // environment alias, e.g. cka3962; empty for the terminal
	Role  string // control-plane | worker | worker2 | host | terminal
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
	// CreateEnv creates the environment named alias (a cluster with its nodes
	// prepared, or a Linux container) and returns when it is usable.
	CreateEnv(ctx context.Context, h dockerhost.Host, sessionID, alias string, spec EnvSpec, keys KeyPair, logf func(string, ...any)) error
	// EnsureTerminal (re)creates the terminal container wired to the given environments.
	EnsureTerminal(ctx context.Context, h dockerhost.Host, sessionID string, envs []Env, keys KeyPair) error
	Exec(ctx context.Context, h dockerhost.Host, sessionID string, t Target, user, script string, env map[string]string) (ExecResult, error)
	Attach(ctx context.Context, h dockerhost.Host, sessionID string, cols, rows uint) (TTY, error)
	// DeleteEnv removes a single environment and leaves the rest of the sandbox alone.
	DeleteEnv(ctx context.Context, h dockerhost.Host, env Env) error
	// Destroy removes the environments, the terminal and whatever else carries
	// the session's label.
	Destroy(ctx context.Context, h dockerhost.Host, sessionID string, envs []Env) error
}

// KeyPair is the per-session ssh identity.
type KeyPair struct {
	Private string `json:"private"`
	Public  string `json:"public"`
}
