# Kubernetes Security Fundamentals

**22% of the exam.** The built-in security mechanisms of the API: who you are, what you may do, what
a Pod may ask for, who may talk to whom, and what gets recorded. Mostly "what happens if" questions.

## The 20% that solves 80%

### Pod Security Standards: three profiles

| Profile | Idea | Forbids, among others |
|---|---|---|
| **Privileged** | Unrestricted, for system and infrastructure workloads | nothing |
| **Baseline** | Prevents known privilege escalations, allows the default Pod | `privileged`, host namespaces, `hostPath`, host ports, added capabilities, `Unconfined` seccomp |
| **Restricted** | Current hardening best practice | everything in Baseline, plus: running as root, `allowPrivilegeEscalation` not false, capabilities not dropped (`ALL`), seccomp profile not set, most volume types |

Running as root is allowed by Baseline. Restricted is the only profile that demands *positive*
settings in the Pod spec, so a plain `kubectl run` Pod passes Baseline and fails Restricted.

### Pod Security Admission: three modes, set by namespace labels

```yaml
metadata:
  labels:
    pod-security.kubernetes.io/enforce: baseline
    pod-security.kubernetes.io/warn: restricted
    pod-security.kubernetes.io/audit: restricted
```

| Mode | On violation | Applies to |
|---|---|---|
| `enforce` | Pod is rejected | Pods only |
| `warn` | Warning to the user, request allowed | Pods and workload resources |
| `audit` | Annotation in the audit log, request allowed | Pods and workload resources |

- A Deployment that violates `enforce` is still created; its Pods are not. Look at the ReplicaSet.
- PSA validates and never changes a Pod. Exemptions (users, RuntimeClasses, namespaces) are set in
  the admission configuration, not by labels.

### Authentication

- Kubernetes has **no User objects**. Normal users come from outside: client certificates, OIDC
  tokens, an authenticating proxy, webhook token authentication. ServiceAccounts are API objects.
- X.509: `CN` is the user name, each `O` a group. Certificates cannot be revoked individually.
- ServiceAccount tokens in Pods are short-lived, bound to the Pod and rotated by the kubelet. The
  user name is `system:serviceaccount:<namespace>:<name>`.
- Special: `system:anonymous`, `system:authenticated`, `system:masters` (bypasses authorization).

### Authorization

- Modes are asked in order (`Node,RBAC`): the first *allow* or *deny* decides; if none has an
  opinion the request is denied with 403.
- RBAC is **additive only**: no deny rules, no order. What no rule allows is forbidden.

| Rules in | Bound with | Effect |
|---|---|---|
| Role | RoleBinding | In that namespace |
| ClusterRole | RoleBinding | In the RoleBinding's namespace only |
| ClusterRole | ClusterRoleBinding | In all namespaces and for cluster-scoped resources |

- Verbs that escalate: `escalate` and `bind` on roles, `impersonate`, `create` on Pods (reads any
  Secret of the namespace), `get`/`list`/`watch` on Secrets, wildcards.

### Secrets

- base64 is an encoding, not encryption. By default Secrets are stored **unencrypted** in etcd.
- Encryption at rest: `--encryption-provider-config` on the API server; the first provider in the
  list encrypts, `identity` means no encryption. A KMS provider keeps the key outside the cluster.
- Whoever may create Pods in a namespace can read all its Secrets by mounting them. Restrict with
  least-privilege RBAC and consider an external secret store.

### Isolation and segmentation

- **Namespaces** scope names, RBAC, quotas, Pod Security and NetworkPolicies. On their own they
  isolate neither network nor nodes. **ResourceQuota** and **LimitRange** keep tenants from
  starving each other.
- **Node isolation:** label nodes with a `node-restriction.kubernetes.io/` prefix, which
  NodeRestriction keeps kubelets from setting, and select on those labels. Stronger still:
  sandboxed runtimes through RuntimeClass, or separate clusters.

### NetworkPolicy

1. No policy selects a Pod: all traffic is allowed, in both directions.
2. A policy selects a Pod for a direction (`policyTypes`): that direction is denied except what is listed.
3. Policies are **additive**: the union of all allow lists applies. No deny rules, no priorities.

`podSelector: {}` with `policyTypes: [Ingress]` and no rules is the default deny for a namespace.
Policies are namespaced, work at layer 3 and 4, and need a network plugin that implements them. A
connection between two isolated Pods needs egress on one side and ingress on the other.

### Audit logging

| Level | Records |
|---|---|
| `None` | nothing |
| `Metadata` | user, time, resource, verb; no bodies |
| `Request` | metadata and request body |
| `RequestResponse` | metadata, request body and response body |

Stages: `RequestReceived`, `ResponseStarted` (long-running requests only), `ResponseComplete`,
`Panic`. The **first matching rule** of the policy sets the level; without `--audit-policy-file`
nothing is logged. Backends: log file or webhook. Use `Metadata` for Secrets, or the log holds them.

## Common confusions

- **Standards vs Admission:** the Standards define the profiles, Admission is the controller that
  applies them per namespace.
- **Authentication vs authorization vs admission:** who are you, may you do this, is this object
  acceptable. Only admission sees the content of the object.
- **A ClusterRole is not automatically cluster-wide.** The binding decides the scope.
- **An empty `podSelector` selects all Pods; an empty `ingress` list allows nothing.**
