package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Values with what a shell or printf could trip over.
const (
	testKey = "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAA+/SECRETKEYMATERIAL==\n-----END OPENSSH PRIVATE KEY-----\n"
	testKC0 = "apiVersion: v1\nclusters:\n- cluster:\n    certificate-authority-data: Q0FEQVRB\n    server: https://cka1234-control-plane:6443\n" +
		"users:\n- user:\n    client-key-data: U0VDUkVUQ0xJRU5US0VZ\n"
	testKC1 = "  leading blanks, a tab\there, a backslash \\n \\0 %s %d, $HOME `id` $(id) \"quoted\" 'single' *\n\n-trailing: two empty lines\n\n\n"
)

// The session's ssh key and the kubeconfigs of its clusters must not be
// arguments of a process: `ps` shows those to every user of the machine. In
// hosted mode that machine is the server's Pod.
func TestTerminalSetupKeepsSecretsOffTheCommandLine(t *testing.T) {
	cfg := sshConfig([]Env{{Alias: "cka1234", Type: EnvCluster}})
	args, stdin := terminalSetup("kd-abcd2345-terminal", testKey, cfg, []string{testKC0, testKC1})

	head := []string{"exec", "-i", "-u", "candidate", "kd-abcd2345-terminal", "bash", "-c"}
	if len(args) != len(head)+1 || !slices.Equal(args[:len(head)], head) {
		t.Fatalf("args = %q, want %q and the script", args, head)
	}
	argv := strings.Join(args, "\x00")
	for name, secret := range map[string]string{
		"the private key":            "SECRETKEYMATERIAL",
		"a line of the private key":  "BEGIN OPENSSH PRIVATE KEY",
		"a client key":               "U0VDUkVUQ0xJRU5US0VZ",
		"a kubeconfig":               "certificate-authority-data",
		"the second kubeconfig":      "leading blanks",
		"an environment assignment":  "KD_PRIV=",
		"a kubeconfig in a variable": "KD_KC_0=",
	} {
		if strings.Contains(argv, secret) {
			t.Errorf("the arguments of docker exec contain %s (%q)", name, secret)
		}
	}
	if slices.Contains(args, "-e") || slices.Contains(args, "--env") {
		t.Errorf("docker exec gets environment variables: %q", args[:len(head)])
	}
	// They are on stdin, one field each, in the order the script reads them.
	want := strings.TrimRight(testKey, "\n") + "\x00" + cfg + "\x00" + testKC0 + "\x00" + testKC1 + "\x00"
	if stdin != want {
		t.Errorf("stdin =\n%q\nwant\n%q", stdin, want)
	}
	script := args[len(args)-1]
	for i, read := range []string{"KD_PRIV", "KD_SSHCFG", "KD_KC_0", "KD_KC_1"} {
		line := "IFS= read -r -d '' " + read + "\n"
		if strings.Count(script, line) != 1 {
			t.Fatalf("the script does not read %s exactly once:\n%s", read, script)
		}
		if i > 0 && strings.Index(script, line) < strings.Index(script, "IFS= read -r -d '' KD_PRIV\n") {
			t.Errorf("the script reads %s before the key", read)
		}
	}
}

// The script, run by a real bash with a fake home: the files have exactly the
// content they had when the values came in as environment variables, byte for
// byte.
func TestTerminalSetupScript(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("no bash on this machine")
	}
	for name, kubeconfigs := range map[string][]string{
		"two clusters": {testKC0, testKC1},
		"one cluster":  {testKC1},
		"no cluster":   nil, // a Linux track
	} {
		t.Run(name, func(t *testing.T) {
			home, bin := t.TempDir(), t.TempDir()
			// What the script calls and this machine must not run or may not have.
			for tool, body := range map[string]string{
				"sudo":    "#!/bin/sh\necho \"$@\" >> \"$HOME/sudo.log\"\n",
				"kubectl": "#!/bin/sh\necho \"flattened $KUBECONFIG\"\n",
			} {
				if err := os.WriteFile(filepath.Join(bin, tool), []byte(body), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			// A kubeconfig of an earlier call that is no longer wanted.
			parts := filepath.Join(home, ".kube", "parts")
			if err := os.MkdirAll(parts, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(parts, "7.yaml"), []byte("stale"), 0o644); err != nil {
				t.Fatal(err)
			}

			cfg := sshConfig([]Env{{Alias: "cka1234", Type: EnvCluster}, {Alias: "linux4821", Type: EnvContainer}})
			args, stdin := terminalSetup("kd-s1-terminal", testKey, cfg, kubeconfigs)
			cmd := exec.Command(bash, "-c", args[len(args)-1])
			cmd.Env = []string{"HOME=" + home, "PATH=" + bin + ":/usr/bin:/bin"}
			cmd.Stdin = strings.NewReader(stdin)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("the script failed: %v\n%s", err, out)
			}

			want := map[string]string{
				".ssh/id_ed25519": testKey, // its trailing newline is cut and written again
				".ssh/config":     cfg + "\n",
				"sudo.log":        "mkdir -p /opt/course\nchown candidate:candidate /opt/course\n",
			}
			var joined []string
			for i, kc := range kubeconfigs {
				part := filepath.Join(".kube", "parts", string(rune('0'+i))+".yaml")
				want[part] = kc + "\n"
				joined = append(joined, filepath.Join(home, part))
			}
			if len(kubeconfigs) > 0 {
				want[".kube/config"] = "flattened " + strings.Join(joined, ":") + ":\n"
			}
			for file, content := range want {
				got, err := os.ReadFile(filepath.Join(home, file))
				if err != nil {
					t.Errorf("%s: %v", file, err)
				} else if string(got) != content {
					t.Errorf("%s =\n%q\nwant\n%q", file, got, content)
				}
			}
			for _, file := range []string{".ssh/id_ed25519", ".ssh/config"} {
				if fi, err := os.Stat(filepath.Join(home, file)); err == nil && fi.Mode().Perm() != 0o600 {
					t.Errorf("%s has mode %o, want 600", file, fi.Mode().Perm())
				}
			}
			left, _ := filepath.Glob(filepath.Join(parts, "*"))
			if len(left) != len(kubeconfigs) {
				t.Errorf("kubeconfig parts: %v, want %d and not the stale one", left, len(kubeconfigs))
			}
			if _, err := os.Stat(filepath.Join(home, ".kube", "config")); len(kubeconfigs) == 0 && err == nil {
				t.Error("a kubeconfig was written for a session without a cluster")
			}
		})
	}

	// A field that is missing ends the script before it writes a key file.
	home := t.TempDir()
	args, stdin := terminalSetup("kd-s1-terminal", testKey, "cfg", []string{testKC0})
	cmd := exec.Command(bash, "-c", args[len(args)-1])
	cmd.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin"}
	cmd.Stdin = strings.NewReader(strings.TrimSuffix(stdin, testKC0+"\x00"))
	if err := cmd.Run(); err == nil {
		t.Error("the script succeeded although a kubeconfig was missing on stdin")
	}
	if _, err := os.Stat(filepath.Join(home, ".ssh", "id_ed25519")); err == nil {
		t.Error("the script wrote the key although its input was incomplete")
	}
}
