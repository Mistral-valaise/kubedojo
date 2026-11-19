package sandbox

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/dockerhost"
)

// A container environment is one Linux container, the "lab": the host the
// learner reaches with `ssh <alias>` from the terminal. It needs no cluster,
// so it lives on a network of its own instead of kind's.

const (
	// sessionLabel marks every container and network that belongs to a session
	// and is not part of a kind cluster. Teardown goes by it.
	sessionLabel = "kubedojo.session"
	// kindNetwork is the Docker network kind creates for its clusters. It does
	// not exist on a machine that never ran kind.
	kindNetwork = "kind"
)

// sessionNetwork is the bridge that connects the terminal with the labs of a
// session. One per session: labs of different sessions cannot reach each other.
func sessionNetwork(sessionID string) string { return "kd-" + sessionID }

// terminalNetwork is the network the terminal container joins: kind's when
// the session has a cluster, the session's own bridge otherwise.
func terminalNetwork(sessionID string, envs []Env) string {
	for _, e := range envs {
		if e.Type != EnvContainer {
			return kindNetwork
		}
	}
	return sessionNetwork(sessionID)
}

// labRunArgs builds the `docker run` arguments of a lab container.
//
// The limits are what keeps one learner's fork bomb or runaway `yes` from
// taking the machine (or the other sessions on it) down. The container gets
// Docker's default capabilities and nothing more: it is not privileged.
func labRunArgs(sessionID, alias, image, pubKey string) []string {
	return []string{"run", "-d",
		// An init process reaps what exercises about processes leave behind.
		"--init",
		"--name", alias, "--hostname", alias,
		"--network", sessionNetwork(sessionID),
		"--label", sessionLabel + "=" + sessionID,
		"--memory", "1g", "--memory-swap", "1g", "--pids-limit", "512", "--cpus", "2",
		"--restart", "unless-stopped",
		// The one capability beyond Docker's defaults: without it not even root
		// can see which process owns another user's socket or open file, and
		// `sudo ss -ltnp` and `sudo lsof` show less than on a real host. It
		// reaches the processes of this container only.
		"--cap-add", "SYS_PTRACE",
		// Read by the image's entrypoint on every start, so sshd and the key
		// survive a restart of the container or of Docker.
		"-e", "KD_PUBKEY=" + pubKey,
		image}
}

// ensureNetwork creates the session's bridge unless it exists.
func ensureNetwork(ctx context.Context, h dockerhost.Host, sessionID string) error {
	name := sessionNetwork(sessionID)
	if _, err := run(h.Command(ctx, "docker", "network", "inspect", "-f", "{{.Name}}", name), ""); err == nil {
		return nil
	}
	_, err := run(h.Command(ctx, "docker", "network", "create", "--label", sessionLabel+"="+sessionID, name), "")
	return err
}

func (k *Kind) createLab(ctx context.Context, h dockerhost.Host, sessionID, alias string, keys KeyPair, logf func(string, ...any)) error {
	logf("Starting host %s ...", alias)
	if err := ensureNetwork(ctx, h, sessionID); err != nil {
		return err
	}
	if _, err := run(h.Command(ctx, "docker", labRunArgs(sessionID, alias, k.LinuxImage, keys.Public)...), ""); err != nil {
		return err
	}
	// The learner's first command is `ssh <alias>`: wait for sshd's banner.
	deadline := time.Now().Add(60 * time.Second)
	for {
		_, err := run(h.Command(ctx, "docker", "exec", alias, "bash", "-c",
			`exec 3<>/dev/tcp/127.0.0.1/22 && read -r -t 2 banner <&3 && [[ $banner == SSH-* ]]`), "")
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return fmt.Errorf("host %s does not accept ssh: %w", alias, err)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// removeLabelled removes every container of the session that is not a kind
// node, and then the session's network, which Docker only removes once
// nothing is attached to it.
func removeLabelled(ctx context.Context, h dockerhost.Host, sessionID string) error {
	filter := "label=" + sessionLabel + "=" + sessionID
	ids, err := run(h.Command(ctx, "docker", "ps", "-aq", "--filter", filter), "")
	if err != nil {
		return err
	}
	if f := strings.Fields(ids); len(f) > 0 {
		if _, err := run(h.Command(ctx, "docker", append([]string{"rm", "-f", "-v"}, f...)...), ""); err != nil {
			return err
		}
	}
	nets, err := run(h.Command(ctx, "docker", "network", "ls", "-q", "--filter", filter), "")
	if err != nil {
		return err
	}
	if f := strings.Fields(nets); len(f) > 0 {
		_, err = run(h.Command(ctx, "docker", append([]string{"network", "rm"}, f...)...), "")
	}
	return err
}
