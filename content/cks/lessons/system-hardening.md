# System Hardening

**10% of the exam.** The node is the last line of defence: less software, fewer privileges, kernel filters.

## The 20% that solves 80%

### Reduce the attack surface of the host

```bash
ss -ltnp                          # listening TCP sockets with PID and program
systemctl list-units --type=service --state=running
systemctl cat <unit>              # what does it start?
systemctl disable --now <unit>    # stop it now and at boot
apt-get purge <package>           # remove what is not needed
```

For an unknown process: `ls -l /proc/<pid>/exe` shows its binary, `cat /proc/<pid>/cmdline | tr '\0' ' '`
its arguments. Remove users, packages and services a Kubernetes node does not need.

### seccomp — filter system calls

A seccomp profile is a JSON file listing which syscalls are allowed, logged or denied.

- `RuntimeDefault` — the container runtime's built-in profile. Use it unless a task says otherwise.
- `Localhost` — your own profile, as a file **on the node**, relative to `/var/lib/kubelet/seccomp/`.

```yaml
spec:
  securityContext:
    seccompProfile:
      type: Localhost
      localhostProfile: profiles/audit.json     # /var/lib/kubelet/seccomp/profiles/audit.json
```

The file must exist on every node the Pod can land on. A missing profile gives
`CreateContainerError: failed to create containerd container: cannot load seccomp profile`.

### AppArmor — restrict files, capabilities, network per program

```bash
apparmor_parser -q /etc/apparmor.d/my-profile     # load on the node
aa-status | grep my-profile
```

```yaml
securityContext:
  appArmorProfile:
    type: Localhost
    localhostProfile: my-profile                  # profile name, not a path
```

Like seccomp, the profile is node-local: load it on the node first.

### Sandboxed runtimes

A `RuntimeClass` selects a different runtime handler (for example gVisor's `runsc`) for a Pod:

```yaml
apiVersion: node.k8s.io/v1
kind: RuntimeClass
metadata: {name: gvisor}
handler: runsc
---
spec:
  runtimeClassName: gvisor
```

### Least privilege for workloads on the node

Avoid `hostNetwork`, `hostPID`, `hostIPC`, `hostPath` and `privileged: true`. Each of them removes one
wall between the container and the node.

## Habits that save points

- Kernel-level tasks are solved **on the node** named in the task — ssh there first.
- After loading or copying a profile, create the Pod and check it actually runs:
  `k describe pod` shows profile errors in its Events.
