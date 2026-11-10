**1. Put the profile on the node.** The Pod will run on the worker, so the file must be there:

```bash
ssh {{host}}
scp /opt/course/seccomp/deny-dangerous.json {{worker}}:/tmp/
ssh {{worker}}
sudo mkdir -p /var/lib/kubelet/seccomp/profiles
sudo mv /tmp/deny-dangerous.json /var/lib/kubelet/seccomp/profiles/
exit
```

**2. Reference it** — relative to `/var/lib/kubelet/seccomp`:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: hardened
  namespace: sec-lark
spec:
  securityContext:
    seccompProfile:
      type: Localhost
      localhostProfile: profiles/deny-dangerous.json
  containers:
    - name: nginx
      image: nginx:1.27-alpine
```

```bash
k apply -f pod.yaml
k -n sec-lark get pod hardened                               # Running
k -n sec-lark exec hardened -- grep Seccomp /proc/1/status   # Seccomp: 2
```

If the Pod shows `CreateContainerError`, `k -n sec-lark describe pod hardened` says
`cannot load seccomp profile ... no such file or directory` — wrong path or wrong node.

**Profile anatomy:** `defaultAction` applies to every syscall not listed (`SCMP_ACT_ALLOW`,
`SCMP_ACT_ERRNO`, `SCMP_ACT_LOG`); `syscalls` lists exceptions. A deny-by-default profile
(`defaultAction: SCMP_ACT_ERRNO` plus an allow list) is stricter but must contain every syscall the
application needs.
