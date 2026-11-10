package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/kubedojo/kubedojo/internal/dockerhost"
	"github.com/moby/moby/client"
)

// Kind is the container-based driver: every node is a kindest/node container
// running real systemd + kubeadm + kubelet + etcd.
type Kind struct {
	NodeImage     string
	TerminalImage string
	KindBin       string // defaults to "kind"
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
		return out.String(), fmt.Errorf("%s: %w: %s", cmd.Args[0], err, lastLines(msg, 6))
	}
	return out.String(), nil
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
		if _, err := run(h.Command(ctx, "docker", "run", "-d", "--name", name, "--hostname", "terminal",
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
	args := []string{"exec", "-i"}
	if user != "" {
		args = append(args, "-u", user)
	}
	for key, v := range env {
		args = append(args, "-e", key+"="+v)
	}
	args = append(args, containerFor(sessionID, t), "bash", "-s")
	cmd := h.Command(ctx, "docker", args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	cmd.Stdin = strings.NewReader(script)
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
}

func (t *tty) Read(p []byte) (int, error)  { return t.conn.Read(p) }
func (t *tty) Write(p []byte) (int, error) { return t.conn.Write(p) }
func (t *tty) Close() error                { err := t.conn.Close(); t.closer(); return err }
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
	ex, err := cli.ExecCreate(ctx, terminalName(sessionID), client.ExecCreateOptions{
		User: "candidate", TTY: true, AttachStdin: true, AttachStdout: true, AttachStderr: true,
		WorkingDir: "/home/candidate", ConsoleSize: size,
		Env: []string{"TERM=xterm-256color", "LANG=C.UTF-8"},
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
		closer: func() { cli.Close() },
	}, nil
}

func (k *Kind) Destroy(ctx context.Context, h dockerhost.Host, sessionID string, aliases []string) error {
	var errs []error
	for _, a := range aliases {
		if _, err := run(h.Command(ctx, k.kind(), "delete", "cluster", "--name", a), ""); err != nil {
			errs = append(errs, err)
		}
	}
	_, _ = run(h.Command(ctx, "docker", "rm", "-f", terminalName(sessionID)), "")
	return errors.Join(errs...)
}
