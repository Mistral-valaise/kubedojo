package sandbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/dockerhost"
	"github.com/moby/moby/client"
)

// Kind is the container-based driver: every node is a kindest/node container
// running real systemd + kubeadm + kubelet + etcd.
type Kind struct {
	NodeImage     string
	TerminalImage string
	KindBin       string // defaults to "kind"

	// RegistryCache starts a pull-through cache for Docker Hub next to the
	// clusters and points every node at it, so images are downloaded once
	// instead of once per cluster and rate limits are not hit.
	RegistryCache bool
	// RegistryMirror is the URL of an existing Docker Hub mirror to use
	// instead (hosted mode), e.g. http://registry-cache.kubedojo.svc:5000.
	RegistryMirror string
}

const cacheContainer = "kubedojo-registry-cache"

// mirror returns the Docker Hub mirror nodes should use, starting the local
// cache when needed. An empty result means "pull directly".
func (k *Kind) mirror(ctx context.Context, h dockerhost.Host, logf func(string, ...any)) string {
	if k.RegistryMirror != "" {
		return k.RegistryMirror
	}
	if !k.RegistryCache {
		return ""
	}
	state, _ := run(h.Command(ctx, "docker", "inspect", "-f", "{{.State.Running}}", cacheContainer), "")
	if strings.TrimSpace(state) != "true" {
		_, _ = run(h.Command(ctx, "docker", "rm", "-f", cacheContainer), "")
		if _, err := run(h.Command(ctx, "docker", "run", "-d", "--name", cacheContainer, "--network", "kind",
			"--restart", "unless-stopped", "--label", "kubedojo.component=registry-cache",
			"-v", "kubedojo-registry-cache:/var/lib/registry",
			"-e", "REGISTRY_PROXY_REMOTEURL=https://registry-1.docker.io", "registry:2"), ""); err != nil {
			// The cache is an optimisation; clusters work without it.
			logf("Image cache could not be started, pulling directly: %v", err)
			return ""
		}
	}
	return "http://" + cacheContainer + ":5000"
}

func (k *Kind) Name() string { return "kind" }

func (k *Kind) kind() string {
	if k.KindBin != "" {
		return k.KindBin
	}
	return "kind"
}

func terminalName(sessionID string) string { return "kd-" + sessionID + "-terminal" }

// containerFor maps a Target to a container name. kind names nodes
// <cluster>-control-plane, <cluster>-worker, <cluster>-worker2, ...
func containerFor(sessionID string, t Target) string {
	if t.Role == "terminal" || t.Alias == "" {
		return terminalName(sessionID)
	}
	role := t.Role
	if role == "" {
		role = "control-plane"
	}
	return t.Alias + "-" + role
}

func run(cmd *exec.Cmd, stdin string) (string, error) {
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = strings.TrimSpace(out.String())
		}
		return out.String(), fmt.Errorf("%s: %w: %s", cmd.Args[0], err, errorSummary(msg))
	}
	return out.String(), nil
}

// errorSummary picks the lines that explain a failure. kind prints "ERROR:"
// lines followed by a long stack trace, so the tail alone says nothing.
func errorSummary(msg string) string {
	var hits []string
	for _, l := range strings.Split(msg, "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "ERROR") || strings.Contains(t, "too many open files") || strings.Contains(t, "no space left") {
			hits = append(hits, t)
		}
	}
	if len(hits) > 0 {
		if len(hits) > 4 {
			hits = hits[:4]
		}
		return strings.Join(hits, " | ")
	}
	return lastLines(msg, 6)
}

func lastLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func (k *Kind) CreateEnv(ctx context.Context, h dockerhost.Host, sessionID, alias string, spec EnvSpec, keys KeyPair, logf func(string, ...any)) error {
	cfg := spec.KindConfig
	if cfg == "" {
		var b strings.Builder
		b.WriteString("kind: Cluster\napiVersion: kind.x-k8s.io/v1alpha4\nnodes:\n- role: control-plane\n")
		for i := 0; i < spec.Workers; i++ {
			b.WriteString("- role: worker\n")
		}
		cfg = b.String()
	}
	f, err := os.CreateTemp("", "kubedojo-kind-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(cfg); err != nil {
		return err
	}
	f.Close()
	kc, err := os.CreateTemp("", "kubedojo-kc-*")
	if err != nil {
		return err
	}
	kc.Close()
	defer os.Remove(kc.Name())

	logf("Creating cluster %s (1 control plane, %d worker) ...", alias, spec.Workers)
	if _, err := run(h.Command(ctx, k.kind(), "create", "cluster", "--name", alias,
		"--image", k.NodeImage, "--config", f.Name(), "--kubeconfig", kc.Name(), "--wait", "120s"), ""); err != nil {
		return err
	}
	nodes, err := k.nodes(ctx, h, alias)
	if err != nil {
		return err
	}
	logf("Preparing %d node(s) of %s (ssh, candidate user, kubeconfig) ...", len(nodes), alias)
	for _, n := range nodes {
		if _, err := run(h.Command(ctx, "docker", "exec", "-i", "-e", "KD_PUBKEY="+keys.Public, n, "bash", "-c",
			`kubedojo-node-init "$KD_PUBKEY" && cat > /root/.ssh/id_ed25519 && chmod 600 /root/.ssh/id_ed25519 && printf 'Host *\n  StrictHostKeyChecking no\n  UserKnownHostsFile /dev/null\n  LogLevel ERROR\n' > /root/.ssh/config`),
			keys.Private); err != nil {
			return fmt.Errorf("init node %s: %w", n, err)
		}
	}
	if m := k.mirror(ctx, h, logf); m != "" {
		// containerd reads hosts.toml on every pull, so no restart is needed.
		// If the mirror is down it falls back to Docker Hub itself.
		hosts := fmt.Sprintf("server = \"https://registry-1.docker.io\"\n\n[host.%q]\n  capabilities = [\"pull\", \"resolve\"]\n", m)
		for _, n := range nodes {
			if _, err := run(h.Command(ctx, "docker", "exec", "-i", n, "bash", "-c",
				"mkdir -p /etc/containerd/certs.d/docker.io && cat > /etc/containerd/certs.d/docker.io/hosts.toml"), hosts); err != nil {
				return fmt.Errorf("configure image cache on %s: %w", n, err)
			}
		}
	}
	logf("Waiting for all nodes of %s to become Ready ...", alias)
	if _, err := run(h.Command(ctx, "docker", "exec", alias+"-control-plane", "kubectl", "--kubeconfig", "/etc/kubernetes/admin.conf",
		"wait", "--for=condition=Ready", "nodes", "--all", "--timeout=180s"), ""); err != nil {
		return err
	}
	return nil
}

func (k *Kind) nodes(ctx context.Context, h dockerhost.Host, alias string) ([]string, error) {
	out, err := run(h.Command(ctx, k.kind(), "get", "nodes", "--name", alias), "")
	if err != nil {
		return nil, err
	}
	return strings.Fields(out), nil
}

func (k *Kind) EnsureTerminal(ctx context.Context, h dockerhost.Host, sessionID string, aliases []string, keys KeyPair) error {
	name := terminalName(sessionID)
	state, _ := run(h.Command(ctx, "docker", "inspect", "-f", "{{.State.Running}}", name), "")
	if strings.TrimSpace(state) != "true" {
		_, _ = run(h.Command(ctx, "docker", "rm", "-f", name), "")
		// --init: shells of closed terminals leave orphans behind; without an
		// init process as PID 1 nobody reaps them and they stay as zombies.
		if _, err := run(h.Command(ctx, "docker", "run", "-d", "--init", "--name", name, "--hostname", "terminal",
			"--network", "kind", "--label", "kubedojo.session="+sessionID, "--restart", "unless-stopped",
			k.TerminalImage), ""); err != nil {
			return err
		}
	}
	// ssh identity + host aliases: `ssh cka1234` lands on that cluster's control plane.
	var sshCfg strings.Builder
	for _, a := range aliases {
		fmt.Fprintf(&sshCfg, "Host %s\n  HostName %s-control-plane\n\n", a, a)
	}
	sshCfg.WriteString("Host *\n  User candidate\n  StrictHostKeyChecking no\n  UserKnownHostsFile /dev/null\n  LogLevel ERROR\n")
	// kubeconfig with one context per cluster, for convenience on the terminal.
	var kcs []string
	for _, a := range aliases {
		out, err := run(h.Command(ctx, k.kind(), "get", "kubeconfig", "--internal", "--name", a), "")
		if err != nil {
			return err
		}
		kcs = append(kcs, out)
	}
	script := `set -e
mkdir -p ~/.ssh ~/.kube ~/.kube/parts
rm -f ~/.kube/parts/*
printf '%s\n' "$KD_PRIV" > ~/.ssh/id_ed25519
printf '%s\n' "$KD_SSHCFG" > ~/.ssh/config
chmod 600 ~/.ssh/id_ed25519 ~/.ssh/config
`
	env := []string{"-e", "KD_PRIV=" + strings.TrimRight(keys.Private, "\n"), "-e", "KD_SSHCFG=" + sshCfg.String()}
	for i, kc := range kcs {
		env = append(env, "-e", fmt.Sprintf("KD_KC_%d=%s", i, kc))
		script += fmt.Sprintf("printf '%%s\\n' \"$KD_KC_%d\" > ~/.kube/parts/%d.yaml\n", i, i)
	}
	script += `if ls ~/.kube/parts/*.yaml >/dev/null 2>&1; then
  KUBECONFIG=$(ls ~/.kube/parts/*.yaml | tr '\n' ':') kubectl config view --flatten > ~/.kube/config
  chmod 600 ~/.kube/config
fi
sudo mkdir -p /opt/course && sudo chown candidate:candidate /opt/course
`
	args := append([]string{"exec", "-i", "-u", "candidate"}, env...)
	args = append(args, name, "bash", "-s")
	_, err := run(h.Command(ctx, "docker", args...), script)
	return err
}

func (k *Kind) Exec(ctx context.Context, h dockerhost.Host, sessionID string, t Target, user, script string, env map[string]string) (ExecResult, error) {
	args := []string{"exec"}
	if user != "" {
		args = append(args, "-u", user)
	}
	for key, v := range env {
		args = append(args, "-e", key+"="+v)
	}
	// The script is passed as an argument, not on stdin: commands inside it
	// that read stdin (ssh, kubectl exec -i) would otherwise swallow the rest.
	args = append(args, containerFor(sessionID, t), "bash", "-c", script)
	cmd := h.Command(ctx, "docker", args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	res := ExecResult{Stdout: out.String(), Stderr: errb.String()}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		res.ExitCode = ee.ExitCode()
		return res, nil
	}
	return res, err
}

type tty struct {
	h      dockerhost.Host
	execID string
	conn   interface {
		Read([]byte) (int, error)
		Write([]byte) (int, error)
		Close() error
	}
	resize func(ctx context.Context, cols, rows uint) error
	closer func()
	once   sync.Once
}

func (t *tty) Read(p []byte) (int, error)  { return t.conn.Read(p) }
func (t *tty) Write(p []byte) (int, error) { return t.conn.Write(p) }

// Close ends the shell. It is safe to call more than once.
func (t *tty) Close() error {
	var err error
	t.once.Do(func() {
		err = t.conn.Close()
		t.closer()
	})
	return err
}
func (t *tty) Resize(ctx context.Context, cols, rows uint) error {
	return t.resize(ctx, cols, rows)
}

func (k *Kind) Attach(ctx context.Context, h dockerhost.Host, sessionID string, cols, rows uint) (TTY, error) {
	cli, err := h.Client()
	if err != nil {
		return nil, err
	}
	if cols == 0 {
		cols, rows = 120, 32
	}
	size := client.ConsoleSize{Height: rows, Width: cols}
	// Every process started from this shell inherits the marker, which is how
	// Close finds them: closing the attach stream alone leaves the shell and
	// whatever runs in it alive inside the container.
	raw := make([]byte, 8)
	_, _ = rand.Read(raw)
	marker := "KD_TTY=" + hex.EncodeToString(raw)
	name := terminalName(sessionID)
	ex, err := cli.ExecCreate(ctx, name, client.ExecCreateOptions{
		User: "candidate", TTY: true, AttachStdin: true, AttachStdout: true, AttachStderr: true,
		WorkingDir: "/home/candidate", ConsoleSize: size,
		Env: []string{"TERM=xterm-256color", "LANG=C.UTF-8", marker},
		Cmd: []string{"bash", "-l"},
	})
	if err != nil {
		cli.Close()
		return nil, err
	}
	hj, err := cli.ExecAttach(ctx, ex.ID, client.ExecAttachOptions{TTY: true, ConsoleSize: size})
	if err != nil {
		cli.Close()
		return nil, err
	}
	return &tty{
		h: h, execID: ex.ID, conn: hj.Conn,
		resize: func(ctx context.Context, c, r uint) error {
			_, err := cli.ExecResize(ctx, ex.ID, client.ExecResizeOptions{Height: r, Width: c})
			return err
		},
		closer: func() {
			defer cli.Close()
			// Not the request context: it is usually cancelled by now.
			kctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			hangUp(kctx, cli, name, marker)
		},
	}, nil
}

// hangUpScript hangs up every process that carries the marker of a closed
// terminal, as a real terminal does when its window is closed: SIGHUP for
// all of them (jobs started with nohup survive, as they would anywhere), then
// SIGKILL for a shell that did not leave.
const hangUpScript = `
marked() { for e in /proc/[0-9]*/environ; do tr '\0' '\n' < "$e" 2>/dev/null | grep -qx "$1" && { p=${e#/proc/}; echo "${p%/environ}"; }; done; }
for p in $(marked "$1"); do kill -HUP "$p" 2>/dev/null; done
sleep 1
for p in $(marked "$1"); do [ "$(ps -o sid= -p "$p" 2>/dev/null | tr -d ' ')" = "$p" ] && kill -KILL "$p" 2>/dev/null; done
exit 0
`

func hangUp(ctx context.Context, cli *client.Client, container, marker string) {
	ex, err := cli.ExecCreate(ctx, container, client.ExecCreateOptions{
		// As the shell's own user: without CAP_SYS_PTRACE not even root may
		// read the environment of another user's processes.
		User: "candidate", Cmd: []string{"sh", "-c", hangUpScript, "hangup", marker},
	})
	if err != nil {
		return // the container is gone, and its processes with it
	}
	_, _ = cli.ExecStart(ctx, ex.ID, client.ExecStartOptions{Detach: true})
}

func (k *Kind) DeleteEnv(ctx context.Context, h dockerhost.Host, alias string) error {
	_, err := run(h.Command(ctx, k.kind(), "delete", "cluster", "--name", alias), "")
	return err
}

func (k *Kind) Destroy(ctx context.Context, h dockerhost.Host, sessionID string, aliases []string) error {
	var errs []error
	for _, a := range aliases {
		if err := k.DeleteEnv(ctx, h, a); err != nil {
			errs = append(errs, err)
		}
	}
	_, _ = run(h.Command(ctx, "docker", "rm", "-f", terminalName(sessionID)), "")
	return errors.Join(errs...)
}
