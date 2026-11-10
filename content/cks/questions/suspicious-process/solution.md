**From the process to the Pod** — on the node:

```bash
ssh {{host}}
ssh {{worker}}
sudo -i
ps aux | grep kworkerd
# root   4711  ...  {kworkerd} /bin/sh /tmp/kworkerd
cat /proc/4711/cgroup
# 0::/kubelet.slice/kubelet-kubepods.slice/.../cri-containerd-3f9c1e....scope
crictl ps | grep 3f9c1e
# 3f9c1e...   busybox:1.36   Running   app   0   8a1b...   metrics-relay-7d4c9f6b8-x2x9k
```

The container ID from the cgroup path identifies the container; `crictl ps` shows its Pod in the last
column (or `crictl inspect 3f9c1e | grep io.kubernetes.pod.name`).

**From the Pod to the Deployment and stopping it:**

```bash
exit; exit                                       # back to {{host}}
k -n sec-tern get pod metrics-relay-7d4c9f6b8-x2x9k -o jsonpath='{.metadata.ownerReferences[0].name}'
# metrics-relay-7d4c9f6b8      (ReplicaSet -> Deployment metrics-relay)
echo metrics-relay > /opt/course/forensics/deployment
k -n sec-tern scale deploy metrics-relay --replicas=0
```

**Why not delete?** The Deployment and its ConfigMap show *how* the process got there — here the
entrypoint script drops and starts `/tmp/kworkerd`:

```bash
k -n sec-tern get cm metrics-relay-entrypoint -o yaml
```

A read-only root filesystem and no writable `/tmp` would have prevented exactly this.
