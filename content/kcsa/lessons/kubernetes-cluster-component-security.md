# Kubernetes Cluster Component Security

**22% of the exam.** One question per component, more or less: what does it do, whom does it trust,
and which setting closes its best-known hole. Learn the components as a table of trust relations.

## The 20% that solves 80%

### Who talks to whom

| Component | Listens on | Security in one line |
|---|---|---|
| kube-apiserver | 6443 | The only door: authentication, authorization, admission, audit happen here and only here |
| etcd | 2379 (clients), 2380 (peers) | Holds everything. Only the API server may connect, with mutual TLS |
| kubelet | 10250 | Can run commands in every container of its node. Needs authn and authz of its own |
| kube-scheduler | 10259 | Only assigns Pods to nodes. A client of the API server |
| kube-controller-manager | 10257 | Runs the control loops; holds the CA key and the ServiceAccount signing key |
| kube-proxy | 10256 (health) | Programs Service forwarding on each node. No policy enforcement |
| Container runtime | local socket (CRI) | Whoever reaches the socket controls the node |
| Network plugin (CNI) | - | Pod networking, NetworkPolicy enforcement, possibly encryption |

All of these talk TLS. Every component except the API server is a *client* of the API server and
has an identity and RBAC roles of its own (`system:kube-scheduler`, `system:node:<name>`, ...).

### API server

A request passes **authentication → authorization → admission → (validation) → etcd**.

- Requests without credentials are not dropped by default: they become `system:anonymous` in the
  group `system:unauthenticated` and are then authorized like anyone else. `--anonymous-auth=false`
  switches that off; never bind roles to these subjects without need.
- `--authorization-mode=Node,RBAC` is the usual setting. `AlwaysAllow` disables authorization.
- Admission controllers see the object and can change (mutating) or reject (validating) it. They act
  on writes, not on reads.
- Audit logging and encryption at rest are both configured **on the API server**, by a policy file
  and an encryption configuration file.

### etcd

- Access to etcd bypasses everything the API server enforces: write access is root on the cluster,
  read access reveals every Secret.
- Protect it with TLS for clients and for peers, client certificate authentication
  (`--client-cert-auth`, `--trusted-ca-file`), a firewall, and encrypted, access-controlled backups.
- Without an encryption configuration the API server stores objects in plain text. Encryption at
  rest is done by the API server before writing, not by etcd.

### kubelet

| Setting | Insecure | Hardened |
|---|---|---|
| Anonymous access | `--anonymous-auth=true` | `false`: requests without credentials get 401 |
| Authorization | `AlwaysAllow` | `Webhook`: asks the API server (SubjectAccessReview) |
| Client CA | none | `--client-ca-file`: verifies client certificates |

On the API server side, the **Node authorizer** and the **NodeRestriction** admission plugin limit a
kubelet to its own Node object and to the Pods, Secrets and ConfigMaps bound to its node. A stolen
kubelet credential then exposes one node, not the cluster.

### Controller manager and scheduler

- `--use-service-account-credentials` gives every controller its own ServiceAccount and its own
  `system:controller:*` role instead of one powerful shared identity.
- The scheduler is not a security gate. A Pod with `spec.nodeName` already set never passes through
  it, so placement rules that must hold are enforced at admission.

### Container runtime and Pods

- The kubelet drives the runtime (containerd, CRI-O) through the CRI socket. Mounting that socket
  into a Pod hands the node to the Pod.
- Seccomp is off (`Unconfined`) unless the Pod asks for `RuntimeDefault` or the kubelet runs with
  `--seccomp-default`.
- Pod fields that reach into the node: `hostNetwork`, `hostPID`, `hostIPC`, `privileged`, `hostPath`
  volumes, added capabilities. Each of them removes a piece of the isolation.
- Every Pod gets a ServiceAccount token mounted unless `automountServiceAccountToken: false` is set.
  A Pod that never calls the API does not need one.

### Networking, kube-proxy and storage

- By default every Pod can reach every other Pod, and traffic is not encrypted. NetworkPolicies
  restrict it, **if the network plugin implements them**; otherwise they are stored and ignored.
- kube-proxy implements Services by programming forwarding rules on each node. It is not a firewall.
- Volume contents never pass through the API server, so its encryption at rest does not cover them:
  that is the job of the storage backend or CSI driver. `hostPath` volumes expose the node's
  filesystem and are forbidden from the Baseline profile on.

### Client security

- A kubeconfig holds credentials (certificates, tokens) and can name a command that kubectl executes
  to fetch credentials: protect it like a private key and never use one from an untrusted source.
- Prefer short-lived credentials (OIDC, TokenRequest tokens) over client certificates, which cannot
  be revoked individually. Membership in `system:masters` bypasses authorization altogether.

## Common confusions

- **Encryption at rest is an API server feature**, although the data lies in etcd.
- **Node authorizer vs NodeRestriction:** the authorizer decides what kubelets may read and write at
  all; the admission plugin narrows writes to the kubelet's *own* node and Pods.
- **Kubelet `Webhook` mode** delegates the decision to the API server; the kubelet still answers the
  request itself.
- **kube-proxy does not enforce NetworkPolicy**, and the API server accepting a policy proves nothing.
- **401 vs 403:** 401 means not authenticated, 403 means authenticated but not allowed.
