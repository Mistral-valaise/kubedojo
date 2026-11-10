# Monitoring, Logging & Runtime Security

**20% of the exam.** Notice what happens in the cluster and on its nodes, and keep containers immutable.

## The 20% that solves 80%

### Audit logging

The API server can record every request. Two parts: a **policy** (what to log) and **flags** (where).

```yaml
# /etc/kubernetes/audit/policy.yaml
apiVersion: audit.k8s.io/v1
kind: Policy
rules:
  - level: RequestResponse
    resources:
      - group: ""
        resources: [pods]
  - level: Metadata
    resources:
      - group: ""
        resources: [secrets, configmaps]
  - level: None                       # catch-all: everything else is not logged
```

- **Rules are evaluated top to bottom; the first match wins.** Order matters.
- Levels: `None` < `Metadata` (who, what, when) < `Request` (+ request body) < `RequestResponse`
  (+ response body). Never log Secrets above `Metadata` — the body contains the values.

```yaml
# kube-apiserver.yaml
    - --audit-policy-file=/etc/kubernetes/audit/policy.yaml
    - --audit-log-path=/var/log/kubernetes/audit/audit.log
    - --audit-log-maxage=7
    - --audit-log-maxbackup=2
```

Both the policy file and the log directory need a `hostPath` **volume and volumeMount** in the static
Pod, otherwise the API server cannot see them and will not start.

Reading the log (one JSON object per line):

```bash
jq -c 'select(.objectRef.resource=="secrets") | [.level,.verb,.user.username,.objectRef.name]' audit.log
```

### Find suspicious processes

From the node, every container process is visible:

```bash
ps aux | grep <name>                       # find the PID
cat /proc/<pid>/cgroup                     # contains the Pod UID and container ID
crictl ps                                  # container ID -> Pod name
crictl inspect <container-id> | grep -E '"pid"|io.kubernetes.pod'
crictl pods --name <pod>
```

Or the other way round: `crictl inspect -o go-template --template '{{.info.pid}}' <id>`.

Tools such as **Falco** watch syscalls against rules ("shell spawned in a container", "write below
/etc") and write alerts to their log. Rule changes go into `/etc/falco/falco_rules.local.yaml`.

### Immutable containers

A container that cannot change itself cannot be turned into something else:

```yaml
securityContext:
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
```

plus `emptyDir` volumes for the paths that must be writable. Do not `kubectl exec` fixes into running
containers — change the image or the manifest and roll out.

### Respond without destroying evidence

Scale a compromised Deployment to 0 or isolate its Pods with a NetworkPolicy and a label change, rather
than deleting objects whose spec and logs you still need.

### Audit your workloads

```bash
k get pods -A -o json | jq -r '.items[] | select(any(.spec.containers[]; .securityContext.privileged==true))
  | "\(.metadata.namespace)/\(.metadata.name)"'
```

The same pattern finds `hostNetwork`, `hostPID` and `hostPath` volumes.

## Habits that save points

- Audit flags wrong → the API server does not come back. Look at `crictl logs` of the exited container.
- An empty audit log usually means the catch-all rule comes first, or the volume mount is missing.
