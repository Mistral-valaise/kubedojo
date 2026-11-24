package sandbox

import (
	"fmt"
	"strings"
)

// terminalSetup builds the command that puts the session's ssh key, the ssh
// client configuration and the kubeconfigs of its clusters into the terminal
// container: the arguments of `docker exec`, and what to write to its stdin.
//
// The key and the kubeconfigs (which hold the clusters' admin credentials)
// are secrets. Arguments and the environment of a process can be read by
// every user of the machine (`ps e`), for as long as the process runs, so the
// secrets travel on stdin: one field per value, each ended by a NUL byte,
// which none of them contains. The script reads them in the order they are
// written here.
func terminalSetup(container, privateKey, sshConfig string, kubeconfigs []string) (args []string, stdin string) {
	fields := []string{strings.TrimRight(privateKey, "\n"), sshConfig}
	var script strings.Builder
	script.WriteString(`set -e
IFS= read -r -d '' KD_PRIV
IFS= read -r -d '' KD_SSHCFG
`)
	for i := range kubeconfigs {
		fmt.Fprintf(&script, "IFS= read -r -d '' KD_KC_%d\n", i)
	}
	script.WriteString(`mkdir -p ~/.ssh ~/.kube ~/.kube/parts
rm -f ~/.kube/parts/*
printf '%s\n' "$KD_PRIV" > ~/.ssh/id_ed25519
printf '%s\n' "$KD_SSHCFG" > ~/.ssh/config
chmod 600 ~/.ssh/id_ed25519 ~/.ssh/config
`)
	for i, kc := range kubeconfigs {
		fields = append(fields, kc)
		fmt.Fprintf(&script, "printf '%%s\\n' \"$KD_KC_%d\" > ~/.kube/parts/%d.yaml\n", i, i)
	}
	script.WriteString(`if ls ~/.kube/parts/*.yaml >/dev/null 2>&1; then
  KUBECONFIG=$(ls ~/.kube/parts/*.yaml | tr '\n' ':') kubectl config view --flatten > ~/.kube/config
  chmod 600 ~/.kube/config
fi
sudo mkdir -p /opt/course && sudo chown candidate:candidate /opt/course
`)
	// -i: stdin is connected. The script is an argument: it holds no secret.
	args = []string{"exec", "-i", "-u", "candidate", container, "bash", "-c", script.String()}
	return args, strings.Join(fields, "\x00") + "\x00"
}
