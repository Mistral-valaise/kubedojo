package sandbox

import (
	"slices"
	"strings"
	"testing"
)

// The lab container is where a learner runs arbitrary commands as root. Its
// limits and the absence of privileges are the contract; a change to these
// arguments should be a decision, not an accident.
func TestLabRunArgs(t *testing.T) {
	got := labRunArgs("abcd2345", "linux4821", "kubedojo/linux:trixie", "ssh-ed25519 AAAA kubedojo")
	want := []string{"run", "-d", "--init",
		"--name", "linux4821", "--hostname", "linux4821",
		"--network", "kd-abcd2345",
		"--label", "kubedojo.session=abcd2345",
		"--memory", "1g", "--memory-swap", "1g", "--pids-limit", "512", "--cpus", "2",
		"--restart", "unless-stopped",
		"--cap-add", "SYS_PTRACE",
		"-e", "KD_PUBKEY=ssh-ed25519 AAAA kubedojo",
		"kubedojo/linux:trixie"}
	if !slices.Equal(got, want) {
		t.Errorf("labRunArgs =\n  %q\nwant\n  %q", got, want)
	}
	if n := strings.Count(strings.Join(got, " "), "--cap-add"); n != 1 {
		t.Errorf("the lab container gets %d capabilities added, want only SYS_PTRACE", n)
	}
	for _, a := range got {
		if a == "--privileged" || strings.HasPrefix(a, "--cap-add=") || strings.HasPrefix(a, "--security-opt") ||
			a == "-v" || strings.HasPrefix(a, "--volume") || strings.HasPrefix(a, "--mount") || a == "--pid" || strings.HasPrefix(a, "--pid=") {
			t.Errorf("the lab container must not get %q", a)
		}
	}
	if got[len(got)-1] != "kubedojo/linux:trixie" {
		t.Error("the image must be the last argument: nothing after it may become the command")
	}
}

func TestContainerFor(t *testing.T) {
	for _, c := range []struct {
		target Target
		want   string
	}{
		{Target{Alias: "cka1234", Role: RoleControlPlane}, "cka1234-control-plane"},
		{Target{Alias: "cka1234"}, "cka1234-control-plane"},
		{Target{Alias: "cka1234", Role: "worker"}, "cka1234-worker"},
		{Target{Alias: "cka1234", Role: "worker2"}, "cka1234-worker2"},
		{Target{Alias: "cka1234", Role: RoleTerminal}, "kd-s1-terminal"},
		{Target{Role: RoleControlPlane}, "kd-s1-terminal"},
		{Target{Alias: "linux4821", Role: RoleHost}, "linux4821"},
		{Target{Alias: "linux4821", Role: RoleTerminal}, "kd-s1-terminal"},
	} {
		if got := containerFor("s1", c.target); got != c.want {
			t.Errorf("containerFor(%+v) = %q, want %q", c.target, got, c.want)
		}
	}
}

// The kind network exists only where kind has run: a session without a
// cluster must not ask for it.
func TestTerminalNetwork(t *testing.T) {
	for name, c := range map[string]struct {
		envs []Env
		want string
	}{
		"a cluster":            {[]Env{{Alias: "cka1", Type: EnvCluster}}, "kind"},
		"an untyped cluster":   {[]Env{{Alias: "cka1"}}, "kind"},
		"a container":          {[]Env{{Alias: "linux1", Type: EnvContainer}}, "kd-s1"},
		"nothing yet":          {nil, "kd-s1"},
		"cluster wins a mix":   {[]Env{{Alias: "linux1", Type: EnvContainer}, {Alias: "cka1", Type: EnvCluster}}, "kind"},
		"two container hosts":  {[]Env{{Alias: "a", Type: EnvContainer}, {Alias: "b", Type: EnvContainer}}, "kd-s1"},
		"session network name": {[]Env{{Alias: "a", Type: EnvContainer}}, sessionNetwork("s1")},
	} {
		if got := terminalNetwork("s1", c.envs); got != c.want {
			t.Errorf("%s: network = %q, want %q", name, got, c.want)
		}
	}
}

func TestSSHConfig(t *testing.T) {
	got := sshConfig([]Env{{Alias: "cka1234", Type: EnvCluster}, {Alias: "linux4821", Type: EnvContainer}})
	for _, want := range []string{
		"Host cka1234\n  HostName cka1234-control-plane\n",
		"Host linux4821\n  HostName linux4821\n",
		"Host *\n  User candidate\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ssh config lacks %q:\n%s", want, got)
		}
	}
}

func TestExecArgs(t *testing.T) {
	env := map[string]string{"KD_QUESTION": "q", "KD_HOST": "linux4821"}
	for name, c := range map[string]struct {
		target Target
		user   string
		want   []string
	}{
		"root on a control plane": {Target{Alias: "cka1", Role: RoleControlPlane}, UserRoot,
			[]string{"exec", "-u", "root", "-e", "KD_HOST=linux4821", "-e", "KD_QUESTION=q", "cka1-control-plane", "bash", "-c", "true"}},
		"root on a host": {Target{Alias: "linux4821", Role: RoleHost}, UserRoot,
			[]string{"exec", "-u", "root", "-e", "KD_HOST=linux4821", "-e", "KD_QUESTION=q", "linux4821", "bash", "-c", "true"}},
		// The learner's account starts where a login would: at home.
		"candidate on a host": {Target{Alias: "linux4821", Role: RoleHost}, UserCandidate,
			[]string{"exec", "-u", "candidate", "-w", "/home/candidate", "-e", "KD_HOST=linux4821", "-e", "KD_QUESTION=q", "linux4821", "bash", "-c", "true"}},
		"candidate on the terminal": {Target{Alias: "linux4821", Role: RoleTerminal}, UserCandidate,
			[]string{"exec", "-u", "candidate", "-e", "KD_HOST=linux4821", "-e", "KD_QUESTION=q", "kd-s1-terminal", "bash", "-c", "true"}},
	} {
		if got := execArgs("s1", c.target, c.user, "true", env); !slices.Equal(got, c.want) {
			t.Errorf("%s:\n  got  %q\n  want %q", name, got, c.want)
		}
	}
}

func TestRolesAndUsersPerEnvironment(t *testing.T) {
	cluster, container := EnvSpec{Type: EnvCluster}, EnvSpec{Type: EnvContainer}
	if cluster.DefaultRole() != RoleControlPlane || container.DefaultRole() != RoleHost {
		t.Errorf("default roles: cluster %q, container %q", cluster.DefaultRole(), container.DefaultRole())
	}
	// Setup, cleanup and checks: root on a machine of the environment, the
	// learner's account on the terminal.
	for _, e := range []EnvSpec{cluster, container} {
		if e.ScriptUser(e.DefaultRole()) != UserRoot || e.ScriptUser(RoleTerminal) != UserCandidate {
			t.Errorf("%s: script users %q / %q", e.Type, e.ScriptUser(e.DefaultRole()), e.ScriptUser(RoleTerminal))
		}
	}
	if cluster.SolveUser() != UserRoot || container.SolveUser() != UserCandidate {
		t.Errorf("solve users: cluster %q, container %q", cluster.SolveUser(), container.SolveUser())
	}
}
