# Container Orchestration

**28% of the exam.** What a cluster does for containers beyond starting them: connect them, protect
them, tell you what went wrong and give them storage. Four sub-topics, each a matter of telling a few
similar things apart.

## The 20% that solves 80%

### Networking

The **network model** is three sentences: every Pod has its own cluster-wide unique IP; every Pod can
reach every other Pod, on any node, without NAT; the containers of one Pod share a network namespace
and talk over `localhost`. Kubernetes only defines this. A pod network implementation, called a
**CNI plugin** after the Container Network Interface, provides it.

Pod IPs change with every new Pod, so clients use a **Service**: a stable name and virtual IP in
front of the Pods that a label selector picks.

| Service type | Reachable from | How |
|---|---|---|
| `ClusterIP` (default) | inside the cluster | virtual cluster IP |
| `NodePort` | outside, via any node | static port on every node (by default from 30000-32767), plus a cluster IP |
| `LoadBalancer` | outside | external load balancer, usually from the cloud provider |
| `ExternalName` | (points outwards) | DNS CNAME to an external hostname, no proxying |

- A **headless** Service (`clusterIP: None`) has no virtual IP; DNS answers with the Pod IPs.
- **DNS**: a Service is `<service>.<namespace>.svc.cluster.local`. The short name works only inside
  the same Namespace; from another one use at least `<service>.<namespace>`.
- **Ingress** routes HTTP(S) from outside to Services by host and path. The object does nothing
  without an **Ingress controller**. The Ingress API is frozen; the project recommends its successor,
  **Gateway API**, with kinds modelled on roles: `GatewayClass` (a kind of gateway and its
  controller), `Gateway` (one instance of traffic handling infrastructure, run by the cluster
  operator), `HTTPRoute` (the routing rules, owned by the application developer).
- **NetworkPolicy** is the firewall between Pods. Without a policy a Pod accepts and sends everything.
  A Pod selected by a policy for a direction is isolated for it, except for what is allowed.
  Policies only add allowances, and the **network plugin** enforces them, if it supports them.

### Security

The **4Cs** (**Cloud**, **Cluster**, **Container**, **Code**) are a layered picture of security, each
layer building on the one outside it. It comes from older versions of the Kubernetes documentation;
the current page goes by lifecycle phase instead: develop, distribute, deploy, runtime.

- **RBAC**: a Role or ClusterRole lists permissions, a binding hands them to users, groups or
  ServiceAccounts. Permissions are additive; there is no deny rule.

  | Binding | refers to | grants in |
  |---|---|---|
  | RoleBinding | Role | its own Namespace |
  | RoleBinding | ClusterRole | its own Namespace |
  | ClusterRoleBinding | ClusterRole | all Namespaces and cluster-scoped resources |

- **ServiceAccount**: the identity of a Pod towards the API server. Every Namespace has one named
  `default`; a Pod that names none uses it and gets a short-lived token mounted. By default it may do
  next to nothing.
- **Secret**: like a ConfigMap, for sensitive values. Base64 is an encoding, not encryption: by
  default Secrets lie unencrypted in etcd. Encryption at rest and tight RBAC make them secret.
- **Pod Security Standards**: `Privileged` (no restrictions), `Baseline` (blocks known privilege
  escalations, a default Pod passes), `Restricted` (hardening best practice). Pod Security Admission
  applies a level per Namespace by label, in the modes `enforce` (reject), `audit` (log) and `warn`.
- **Security context**: settings of the Pod or container itself, such as the user it runs as,
  privileged or not, Linux capabilities, a read-only root filesystem.

### Troubleshooting

Start with `kubectl get pods`, then `kubectl describe pod` for the events, then `kubectl logs`.
"BackOff" always means that Kubernetes keeps trying and waits longer each time.

| You see | It means | Look at |
|---|---|---|
| `Pending`, no node | the scheduler cannot place the Pod (resources, taints, affinity) | events |
| `ImagePullBackOff` / `ErrImagePull` | image name, tag or registry credentials are wrong | events |
| `CrashLoopBackOff` | the container starts, exits, and is restarted with a growing delay | logs, `--previous` |
| `Running`, but `0/1` ready | the readiness probe fails | events, probe settings |

| Probe | Question | On failure |
|---|---|---|
| liveness | is the container still working? | the kubelet restarts the container |
| readiness | can it take traffic now? | the Pod leaves the Service endpoints, no restart |
| startup | has it finished starting? | holds the other two back; then the container is restarted |

### Storage

Files in a container are lost when it restarts. A **volume** belongs to the Pod and survives
container restarts; an ephemeral volume such as `emptyDir` goes when the Pod goes; a **persistent**
volume outlives the Pod.

- **PersistentVolume (PV)**: a piece of storage in the cluster, a cluster resource like a node.
- **PersistentVolumeClaim (PVC)**: a user's request for storage; a Pod mounts the claim, not the PV.
- **StorageClass**: a kind of storage on offer. Its provisioner creates a PV on demand for a claim
  (**dynamic provisioning**) instead of an administrator creating PVs beforehand (static).
- **CSI**, the Container Storage Interface, lets storage vendors supply such drivers out of tree.

| Access mode | Mounted by |
|---|---|
| `ReadWriteOnce` | one **node**, read-write (several Pods on that node may share it) |
| `ReadOnlyMany` | many nodes, read-only |
| `ReadWriteMany` | many nodes, read-write |
| `ReadWriteOncePod` | one single **Pod**, read-write |

**Reclaim policy** decides what happens to a PV when its claim is deleted: `Delete` removes the PV and
the storage behind it (the default of a StorageClass), `Retain` keeps both for manual clean-up.

## Common confusions

- A Service exposes Pods, an Ingress exposes Services, a NetworkPolicy restricts Pods.
- `NodePort` and `LoadBalancer` include a `ClusterIP`; they are layers, not alternatives.
- A failing readiness probe never restarts anything; a failing liveness probe never touches endpoints.
- Pod **phase** is `Pending`, `Running`, `Succeeded`, `Failed` or `Unknown`. `CrashLoopBackOff` and
  `ImagePullBackOff` are not phases but reasons shown in the STATUS column.
