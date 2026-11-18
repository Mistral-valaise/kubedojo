# Kubernetes Threat Model

**16% of the exam.** How an attacker gets from one foothold to the next in a cluster, and which
control cuts each step. Questions describe a situation; you name the weakness or the countermeasure.

## The 20% that solves 80%

### Trust boundaries: the API server is the checkpoint

Every request to the Kubernetes API passes four stages inside the API server:

```
authentication  ->  authorization (RBAC)  ->  admission control  ->  etcd      (+ audit log)
```

A trust boundary is a place where data crosses between parts that trust each other to a different
degree: internet to cluster, user to API server, API server to etcd, API server to kubelet,
container to node, namespace to namespace. Controls sit on the boundary. Whatever goes **around**
the API server is not authorized by RBAC, not checked by admission and not in the audit log:

| Path around the API server | What it gives an attacker | Main defence |
|---|---|---|
| etcd API (port 2379) | read and change all cluster data | own CA for etcd, network restriction, protect client keys |
| kubelet API (port 10250) | Pod list, logs, command execution on that node | no anonymous access, Webhook authorization, restrict the port |
| Static Pod manifests | run Pods without the API server's consent | file permissions and auditing on the node |
| Container runtime socket | start containers, enter running ones | file permissions, never mount it into Pods |

Three details worth knowing: a certificate from the CA that etcd trusts usually means full access,
whatever it was issued for; `get` on `nodes/proxy` is not read-only, because it reaches kubelet
endpoints that execute commands; and a kubelet's own credential is held back by the Node authorizer,
which lets it read only the Secrets and ConfigMaps of Pods bound to its node.

### The attacker's steps and the control for each

| Step | Typical move in Kubernetes | Control |
|---|---|---|
| Malicious code execution | exploit in an application, a poisoned image | minimal images, non-root, read-only root filesystem, seccomp, image scanning |
| Privilege escalation in the container | setuid binary, extra capabilities | `allowPrivilegeEscalation: false`, drop capabilities, `runAsNonRoot` |
| Escape to the node | privileged container, `hostPath`, `hostPID`, host network | Pod Security Standards (Baseline / Restricted), sandboxed runtimes |
| Privilege escalation in the API | powerful RBAC rights, stolen ServiceAccount token | least privilege, no automounted token where none is needed |
| Access to sensitive data | read Secrets, read etcd or its backups | restrict `get`/`list`/`watch`, encryption at rest |
| Attacker on the network | scan and connect to other Pods, sniff traffic | default-deny NetworkPolicies, mTLS |
| Persistence | static Pod, new workload or account, own RoleBinding, backdoored image | node hardening, audit logging, admission policy |
| Denial of service | consume CPU, memory, PIDs or API objects | requests and limits, ResourceQuota, LimitRange |

### RBAC rights that are worth more than they look

- **`list` or `watch` on Secrets** returns the full objects, exactly like `get`.
- **Creating workloads** in a namespace implicitly gives access to every Secret, ConfigMap and
  ServiceAccount of that namespace, because a Pod can mount them and run as any ServiceAccount.
  Boundaries inside a namespace are weak.
- **`escalate`** (on roles) and **`bind`** (on bindings) switch off RBAC's own protection against
  granting rights you do not have. **`impersonate`** lets a user act as someone else.
- **Creating PersistentVolumes** includes `hostPath` volumes, which means node filesystem access.
- **Approving CSRs** for the API server client signer creates new identities, with any name.
- **`create` on `serviceaccounts/token`** issues tokens for existing ServiceAccounts.

### Denial of service

| Object | Scope | Limits |
|---|---|---|
| requests / limits | one container | CPU and memory of that container |
| LimitRange | namespace | defaults, minimum and maximum **per** container, Pod or claim |
| ResourceQuota | namespace | the **sum** of resources and the **number** of objects |

A request that would exceed a ResourceQuota is rejected with 403. Object count quotas are the
answer to someone flooding the API with objects; memory limits protect neighbours on the same node.

### The network as seen by an attacker

Without NetworkPolicies every Pod can reach every other Pod, in every namespace: namespaces are not
a network boundary. A NetworkPolicy only works if the network plugin enforces it; otherwise it is
stored and has no effect. NetworkPolicies filter connections, they do not encrypt them. Against
reading or changing traffic on the wire you need TLS between the workloads, for example mTLS from a
service mesh.

### Compromised containers

Assume the application will be exploited and decide beforehand what the attacker finds:

```yaml
securityContext:
  runAsNonRoot: true                 # kubelet refuses to start the container as UID 0
  allowPrivilegeEscalation: false    # no_new_privs: setuid binaries gain nothing
  readOnlyRootFilesystem: true       # nothing can be written into the container filesystem
  capabilities: {drop: [ALL]}
  seccompProfile: {type: RuntimeDefault}
```

A privileged container is not isolated from its node in any meaningful way: whoever controls it
controls the node, and from there every Pod and every Secret mounted on that node.

## Common confusions

- **Authentication, authorization, admission**: who are you, may you do this, is this object
  acceptable. An attack around the API server skips all three.
- **`runAsNonRoot` and `allowPrivilegeEscalation`**: the first is a check when the container
  starts, the second restricts what processes can become afterwards. You want both.
- **LimitRange and ResourceQuota**: per object against per namespace in total.
- **Static Pod and mirror Pod**: the kubelet runs the static Pod from a file; the mirror Pod is
  only its read-only reflection in the API. A static Pod can run without any mirror Pod.
- **Persistence and privilege escalation**: keeping access against gaining more of it. Creating a
  RoleBinding for yourself can be both.
- **Encryption at rest and RBAC on Secrets**: encryption protects etcd's files and backups; a
  subject that is allowed to read a Secret through the API still gets it decrypted.
- **Base64 is an encoding**, not encryption.
