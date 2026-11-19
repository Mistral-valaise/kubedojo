# Kubernetes Fundamentals

**44% of the exam.** Almost every second question: what the objects are for, which component does
what, how a Pod gets its node and what a container is made of. No YAML to write, but you must tell
neighbouring things apart: most wrong options are the component or object next door.

## The 20% that solves 80%

### Desired state and controllers

You declare a **desired state** in an object's `spec`; a **controller** is a control loop that compares
it with the current state and acts through the API server until both match. Nobody "runs" a
Deployment: a Pod that disappears is replaced because the count is wrong again, not because a command
is repeated. Pods themselves are disposable; you create them through a workload resource.

| Resource | Use it for | Tell it apart by |
| --- | --- | --- |
| Deployment | stateless replicas, rolling updates | manages ReplicaSets (N identical Pods); new Pod template = new ReplicaSet |
| StatefulSet | replicas with identity | names `web-0`, `web-1`; one PVC per Pod; ordered start and stop; needs a headless Service |
| DaemonSet | one Pod per node | follows the nodes, has no replica count |
| Job | a task that runs to completion | finishes, is not restarted for ever |
| CronJob | Jobs on a schedule | creates Job objects, cron syntax |

### Finding things: namespaces, labels, selectors

- **Namespaces** divide names and are the unit for RBAC and quotas. Not everything is namespaced:
  Nodes, PersistentVolumes, Namespaces, ClusterRoles and PriorityClasses are cluster-scoped
  (`kubectl api-resources --namespaced=false`). Initial namespaces: `default`, `kube-system`,
  `kube-public`, `kube-node-lease`.
- **Labels** are key/value pairs for selecting; a **selector** with several requirements is an AND, and
  there is no OR. Services, ReplicaSets and NetworkPolicies all find Pods by label, never by name.
- A **Service** gives a changing set of Pods one stable name and address; the Pods behind it are
  listed in EndpointSlices, which the control plane keeps up to date.
- **ConfigMap** = non-confidential configuration, **Secret** = confidential values. A Secret is only
  base64-encoded and, unless encryption at rest is enabled, stored unencrypted in etcd. Protection
  comes from RBAC and encryption at rest, not from the object type.

### Who does what in the cluster

| Component | Runs on | Job |
| --- | --- | --- |
| kube-apiserver | control plane | exposes the API; stores objects in etcd, which ideally only it can access |
| etcd | control plane | key-value store holding all cluster state: this is what you back up |
| kube-scheduler | control plane | assigns Pods without a node to a node |
| kube-controller-manager | control plane | runs the controllers (Deployment, ReplicaSet, Job, Node, ...) |
| cloud-controller-manager | control plane, optional | talks to the cloud provider (load balancers, nodes, routes) |
| kubelet | every node | makes sure the Pods bound to its node are running |
| kube-proxy | every node, optional | maintains the network rules that implement Services |
| container runtime | every node | pulls images and runs containers (containerd, CRI-O) |

The life of a Pod in one line: controller **creates** it, scheduler **binds** it, kubelet **starts**
it through the runtime, kube-proxy **routes** to it.

### The API and getting access

- Core group: `apiVersion: v1` (Pod, Service, ConfigMap, Secret, Namespace, Node), path `/api/v1`.
  Named groups: `apps/v1` (Deployment, StatefulSet, DaemonSet), `batch/v1` (Job, CronJob),
  `rbac.authorization.k8s.io/v1`, `networking.k8s.io/v1`; path `/apis/<group>/<version>`.
- `kubectl get | describe | apply -f | delete | logs | exec | explain`; `kubectl api-resources` lists
  kinds, short names, group and scope.
- **kubeconfig** (`$HOME/.kube/config` or `$KUBECONFIG`): clusters, users and contexts. A context =
  cluster + user + namespace; `kubectl config use-context <name>` switches.
- **RBAC**: a role lists what is allowed, a binding says for whom (users, groups, ServiceAccounts)
  and where. Only allow rules, no deny.

| | Grants in one namespace | Grants cluster-wide |
| --- | --- | --- |
| Rules defined in | Role, or ClusterRole | ClusterRole |
| Granted with | RoleBinding | ClusterRoleBinding |

### Scheduling

The scheduler **filters** the nodes that can run the Pod, **scores** the rest and binds the Pod to the
best one. A Pod that fits nowhere stays `Pending`.

| Mechanism | Set on | Effect |
| --- | --- | --- |
| requests | container | the sum of requests must fit the node; actual usage is ignored |
| limits | container | not used for placement; enforced at runtime (CPU throttled, memory OOM-killed) |
| `nodeSelector` | Pod | only nodes carrying all the listed labels |
| node affinity | Pod | `required...` = hard, `preferred...` = soft; `IgnoredDuringExecution` = a running Pod stays |
| Pod (anti-)affinity | Pod | near or away from Pods with certain labels |
| taint | node | repels Pods: `NoSchedule`, `PreferNoSchedule` (soft), `NoExecute` (also evicts) |
| toleration | Pod | allows a tainted node, nothing more |
| PriorityClass | Pod | order in the queue; may preempt lower-priority Pods |

### Containers

- An **image** is a packaged filesystem plus metadata, stored in a **registry**. Reference:
  `registry/name:tag` or `registry/name@sha256:<digest>`. No registry = Docker Hub, no tag = `latest`.
  A tag can be moved to another image, a digest is the hash of the content and cannot.
- **OCI** standardizes three things: the image format (image-spec), running an unpacked bundle
  (runtime-spec, implemented by runc) and the registry API (distribution-spec).
- **CRI** is the gRPC interface between kubelet and container runtime. Since v1.24 Kubernetes has no
  built-in Docker Engine integration (dockershim); images built with Docker are OCI images and run
  unchanged.
- A container is an ordinary process with **namespaces** (what it can see: PIDs, network, mounts,
  hostname) and **cgroups** (how much it may use: CPU, memory). It shares the node's kernel; that is
  the difference from a virtual machine. The containers of one Pod share a network namespace: one IP,
  and they reach each other on `localhost`.

## Habits that save points

- "Creates the Pods" = a controller. "Chooses the node" = scheduler. "Starts the containers" =
  kubelet with the runtime. "Stores" = etcd. Read which verb the question uses.
- CRI, CNI, CSI: runtime, network, storage. All three are plugin interfaces; OCI is a standards body.
- Toleration is permission, affinity is attraction. Dedicated nodes need both a taint and a label.
- Requests are for the scheduler, limits for the kernel.
- Encoded is not encrypted; a tag is not a version guarantee; a ClusterRole bound with a RoleBinding
  stays inside that namespace.
