# Minimize Microservice Vulnerabilities

**20% of the exam.** Constrain what a container can do, and protect the Secrets it uses.

## The 20% that solves 80%

### Pod Security Standards

Three levels — `privileged`, `baseline`, `restricted` — applied per Namespace by labels:

```bash
k label ns shop pod-security.kubernetes.io/enforce=restricted
k label ns shop pod-security.kubernetes.io/warn=restricted pod-security.kubernetes.io/audit=restricted
```

`enforce` rejects violating Pods, `warn` and `audit` only report. Existing Pods are not removed; a
Deployment that violates the level simply cannot create new Pods (see `k describe rs`).

A container that passes `restricted`:

```yaml
securityContext:
  runAsNonRoot: true
  allowPrivilegeEscalation: false
  capabilities:
    drop: [ALL]
  seccompProfile:
    type: RuntimeDefault
```

The image must really run as non-root (a numeric `USER` in the image, or `runAsUser`).

### securityContext cheat sheet

| Field | Level | Effect |
|---|---|---|
| `runAsUser`, `runAsGroup`, `fsGroup` | Pod or container | identity of the process and of mounted volumes |
| `runAsNonRoot: true` | Pod or container | refuse to start as UID 0 |
| `readOnlyRootFilesystem: true` | container | image filesystem becomes read-only |
| `allowPrivilegeEscalation: false` | container | blocks setuid binaries and similar |
| `capabilities.drop: [ALL]` | container | removes Linux capabilities |
| `privileged: true` | container | root on the node, effectively — never |

With a read-only root filesystem, give the application `emptyDir` volumes for the few paths it must write
(`/tmp`, cache and pid directories).

### Secrets

- Prefer **mounted files** over environment variables: env vars leak into logs, crash dumps and child
  processes, and `kubectl describe` shows which are set.
- Restrict `get`/`list` on Secrets with RBAC. `list` returns the values too.
- Base64 is not encryption.

### Encryption at rest

```yaml
# /etc/kubernetes/enc/enc.yaml
apiVersion: apiserver.config.k8s.io/v1
kind: EncryptionConfiguration
resources:
  - resources: [secrets]
    providers:
      - aescbc:
          keys:
            - name: key1
              secret: <base64 of 32 random bytes>
      - identity: {}
```

1. `head -c 32 /dev/urandom | base64` for the key.
2. API server flag `--encryption-provider-config=/etc/kubernetes/enc/enc.yaml` **plus** a `hostPath`
   volume and `volumeMount` for that directory.
3. The **first** provider encrypts new writes; all providers are tried for reads.
4. Existing Secrets stay as they are until rewritten: `k get secrets -A -o json | k replace -f -`.

Verify in etcd — the value must start with `k8s:enc:aescbc:v1:key1`:

```bash
ETCDCTL_API=3 etcdctl --cacert /etc/kubernetes/pki/etcd/ca.crt --cert /etc/kubernetes/pki/etcd/server.crt \
  --key /etc/kubernetes/pki/etcd/server.key get /registry/secrets/<ns>/<name> | hexdump -C | head
```

### Isolation between Pods

mTLS between services (service mesh), sandboxed runtimes via `RuntimeClass`, and NetworkPolicies limit
what a compromised container can reach.

## Habits that save points

- Test admission with `--dry-run=server`: it runs the real checks without creating anything.
- When a Deployment has 0 Pods after a security change: `k describe rs` — the reason is in its Events.
