# Overview of Cloud Native Security

**14% of the exam.** The vocabulary of the whole exam: layers, lifecycle phases, isolation and the
supply chain. The questions are about *where* a control belongs and *what kind* of protection it gives.

## The 20% that solves 80%

### The 4Cs: defense in depth, from the outside in

| Layer | What it is | Typical controls |
|---|---|---|
| **Cloud** | Provider, datacenter, network, VMs: the trusted computing base | IAM, network access to the API server and to etcd, disk encryption |
| **Cluster** | Kubernetes components and what runs in the cluster | TLS, authentication, RBAC, admission, NetworkPolicy, Secrets encryption |
| **Container** | Image and runtime of one container | Scanning, signing, non-root user, dropped capabilities, sandboxed runtime |
| **Code** | Your application | TLS between services, dependency analysis, static and dynamic analysis |

- Every layer builds on the one around it. You **cannot** make up for a weak outer layer by working
  on an inner one: perfect code on a cluster with a public, unauthenticated API is still lost.
- The current Kubernetes documentation no longer shows the 4C diagram; it follows the CNCF white
  paper's lifecycle phases instead. The exam curriculum still names the 4Cs: know both.

### Lifecycle phases (CNCF cloud native security white paper)

| Phase | Question it answers | Examples |
|---|---|---|
| **Develop** | Is what we design and write sound? | Threat model, code review, zero trust architecture, fuzzing |
| **Distribute** | Can we trust what we are about to run? | Image scanning, signing, private registry, dependency updates |
| **Deploy** | May this be deployed, by whom, where? | Admission checks, verifying signatures, namespaces |
| **Runtime** | Is what runs kept in bounds? | Access (authn, authz, TLS), compute (Pod Security, sandboxing), storage (encryption) |

Controls are *produced* in one phase and *enforced* in a later one: an image is signed in Distribute
and the signature is verified in Deploy.

### Cloud provider and infrastructure

- **Shared responsibility.** The provider secures what it operates (with managed Kubernetes that
  includes the control plane); you secure what you configure: identities, network exposure, workloads.
- The control plane should not be reachable from the internet; nodes accept connections only from the
  control plane and on the ports they need; etcd is reachable by the API servers only.
- Give the cluster's cloud identity the least privilege that works. The **instance metadata service**
  is reachable from Pods by default and can hand out node credentials: restrict it with network
  policies and keep instance permissions small.

### Isolation techniques: what separates what

| Technique | Separates | Strength |
|---|---|---|
| Linux namespaces | What a process can *see* (PIDs, network, mounts, users) | Shared kernel |
| cgroups | What a process can *use* (CPU, memory, PIDs) | Shared kernel |
| seccomp, AppArmor, SELinux, capabilities | What a process may *do* with the kernel | Shared kernel, smaller attack surface |
| Sandboxed runtime (gVisor, Kata Containers) via **RuntimeClass** | The workload from the host kernel | User-space kernel or lightweight VM |
| Kubernetes Namespace | API objects: names, RBAC, quotas, policies | Not a network or kernel boundary by itself |
| Dedicated nodes, dedicated clusters | Tenants from each other | Strongest, most expensive |

A Kubernetes **Namespace** and a Linux **namespace** are different things with the same name. A
container is a process with namespaces and cgroups on a shared kernel, which is a weaker boundary
than a virtual machine; hostile or untrusted code is the case for sandboxing or separate nodes.

### Artifact repository and image security

- **Tag vs digest.** A tag (`:1.4`, `:latest`) can be moved to other content. A digest
  (`@sha256:...`) is a hash of the content and cannot. Pin by digest when it matters.
- **Scan** images for known vulnerabilities in the pipeline and again in the registry: new CVEs are
  published for images that have not changed.
- **Sign** images and verify the signature at admission; scanning says what is inside, signing says
  where it came from.
- Use a **private registry** with authentication (`imagePullSecrets`), small base images (fewer
  packages, fewer CVEs) and a non-root `USER`.

### Workload and application code security

The `securityContext` settings that turn up again and again:

| Setting | Effect |
|---|---|
| `runAsNonRoot: true` | The kubelet refuses to start a container that would run as UID 0 |
| `allowPrivilegeEscalation: false` | Sets `no_new_privs`: setuid binaries cannot raise privileges. Defaults to **true** |
| `readOnlyRootFilesystem: true` | Root filesystem mounted read-only; writes go to volumes |
| `capabilities.drop: [ALL]` | Removes the root-like capabilities a container gets by default |
| `privileged: true` | Nearly all host access. Almost never right |
| `seccompProfile.type: RuntimeDefault` | The runtime's system call filter instead of none |

For the code itself: encrypt traffic (TLS, or mutual TLS through a service mesh), expose only the
ports that are needed, scan third-party dependencies, and keep credentials out of images and
source code.

## Common confusions

- **Scanning is not signing.** One finds known vulnerabilities, the other proves origin and integrity.
- **`imagePullPolicy: Always` does not pin anything.** It resolves the tag again at every start.
- **`runAsNonRoot` is not `allowPrivilegeEscalation: false`.** The first checks the start UID, the
  second stops a process from gaining privileges later.
- **seccomp and AppArmor harden a container, a sandbox isolates it.** Only the sandboxed runtime
  takes the shared host kernel out of the picture.
- **A hardened inner layer does not rescue an open outer layer.** That is the whole point of the 4Cs.
