# Deploying kubedojo on Kubernetes

Hosted mode is meant for **one team that trusts each other** (single-tenant). Read the security section
before exposing it.

## Architecture

```text
Ingress ─▶ kubedojo server Pod (optional oauth2-proxy sidecar)
              │  creates / deletes            │ Docker API (TCP 2375, NetworkPolicy)
              ▼                               ▼
        Kubernetes API              sandbox Pod per session: Docker-in-Docker
                                      └─ terminal container + kind clusters
```

- The **server** is one Pod with a small PVC (SQLite: sessions, history). It runs as non-root with a
  read-only root filesystem. Its ServiceAccount may create, get, list and delete Pods in its own
  Namespace and read its own Deployment — nothing else. That is still a lot: whoever can create
  Pods in a Namespace that admits privileged ones is root on its nodes.
- Each session gets a **sandbox Pod** (`kd-sandbox-<id>`) that runs a Docker daemon. kubedojo creates
  the same terminal container and kind clusters in it that it creates locally.

## Security

Sandbox Pods are **privileged**: Docker-in-Docker and kind need it. A user with a terminal in a sandbox
should be assumed able to become root on the node that runs the Pod.

1. **Dedicated node pool.** Taint and label nodes for sandboxes and point the chart at them:

   ```bash
   kubectl taint node <node> kubedojo.dev/sandbox=:NoSchedule
   kubectl label node <node> kubedojo.dev/pool=sandbox
   ```
   ```yaml
   sandbox:
     nodeSelector: {kubedojo.dev/pool: sandbox}
     tolerationKey: kubedojo.dev/sandbox
   ```
   Do not run other workloads on these nodes. Ideally they live in their own network segment and have no
   cloud credentials (no instance role, metadata service blocked).

2. **NetworkPolicy** (on by default, needs a CNI that enforces it): only the server can reach a
   sandbox's Docker port, and sandboxes cannot reach private address ranges — your cluster's Pods,
   Services and API server. Internet egress stays open for image pulls.

3. **Authentication.** Without it, everyone who can open the URL gets a shell. Enable OIDC:

   ```bash
   kubectl -n kubedojo create secret generic kubedojo-oidc \
     --from-literal=client-secret=... \
     --from-literal=cookie-secret="$(head -c 32 /dev/urandom | base64)"
   ```
   ```yaml
   oidc:
     enabled: true
     issuerURL: https://login.example.com/realms/main
     clientID: kubedojo
     existingSecret: kubedojo-oidc
     emailDomains: [example.com]
   ```
   Redirect URL to register: `https://<host>/oauth2/callback`. Each user then has their own sessions
   and history.

4. **Pod Security.** The Namespace needs the `privileged` Pod Security level for sandbox Pods.

For a public or multi-tenant service this is not enough isolation. Use VM-based sandboxes (the driver
interface exists; a KubeVirt driver is the planned route) or a sandboxed node runtime.

## Images

| Image | Build |
|---|---|
| server | `make server-image` |
| sandbox host | `make sandbox-host` — Docker-in-Docker with the node, terminal and Linux images preloaded |

Push both to your registry and set `image.repository`, `sandbox.image.repository` and the tags.
The sandbox host image is large (about 1.7 GB) because it carries the cluster images; preloading them
avoids pulling them for every session. Pre-pull it on the sandbox nodes for fast starts.

## Image cache

The chart runs a pull-through cache for Docker Hub (`registryCache.enabled`, on by default) and points
every sandbox cluster at it. The images questions use (`nginx`, `busybox`, ...) are then downloaded
once instead of once per session, and anonymous Docker Hub rate limits are no longer a risk. To use a
mirror you already run, set `registryCache.enabled=false` and `sandbox.registryMirror=http://...`.

Locally, `kubedojo up` starts the same cache as a container (`kubedojo-registry-cache`); disable it
with `--registry-cache=false`.

## Sizing

| | Memory | Notes |
|---|---|---|
| Practice session | 2–3 GB | one or two clusters, created on demand |
| Exam session | 3–5 GB | two clusters and all task setups |
| Session start | about 1 min | about 30 s to start the sandbox host and load its images, then about 35 s per cluster |

### What one sandbox gets

| Value | Default | |
|---|---|---|
| `sandbox.resources.cpuRequest` / `cpuLimit` | `1` / `4` | CPU cores |
| `sandbox.resources.memoryRequest` / `memoryLimit` | `3Gi` / `8Gi` | memory |
| `sandbox.resources.ephemeralStorageRequest` / `ephemeralStorageLimit` | `10Gi` / `30Gi` | the node's disk: Docker's data in an `emptyDir` |

- **Requests are reserved.** The scheduler places a sandbox only on a node that still has the
  requests free. `maxSessions` times the requests must fit, or sandboxes stay `Pending`.
- **Limits are per sandbox, not per node.** With the defaults a sandbox reserves 3 Gi and may use
  8 Gi. Five sandboxes reserve 15 Gi and may together try to use 40 Gi. On a node with less, the
  kernel kills processes in the sandbox that goes over its limit first, or the kubelet evicts Pods
  when the node itself runs out. Size for what sessions really use (the table above), not for the
  requests alone.
- **A limit that is reached ends the session.** Memory: the cluster inside loses containers.
  Ephemeral storage: the kubelet evicts the Pod. kubedojo cannot move a sandbox.
- **An empty limit is no limit.** Requests cannot be empty.
- The server checks the values at start: a request above its limit, less than 1 Gi of memory or
  8 Gi of disk (the preloaded images take about 5 GB of it), or more than 64 cores, 256 Gi of
  memory or 1 Ti of disk stop it with a message. The chart's schema rejects values that are not
  quantities.

Other settings: `maxSessions` caps how many sessions own a sandbox, `maxTerminals` (default 4) caps
the terminal tabs of one session (each is a shell in the sandbox and a WebSocket to the server), and
`idleTTL` (default 3h) removes sandboxes nobody uses.

### How many sessions fit on one node

A single-node k3s host also runs k3s itself, the kubedojo server, the image cache and, if you enable
it, the evaluation observability stack. These numbers are **estimates from the figures above, not
measurements**: about 1.5 GB for k3s and the system, 0.5 GB for kubedojo and the cache, 1 GB for
`lgtm.enabled`, and 4 GB per session as a planning value between practice and exam.

| Host | Memory left for sandboxes | `maxSessions` | `memoryRequest` / `memoryLimit` | Disk for sandboxes |
|---|---|---|---|---|
| 16 GB, 4 cores | about 13 GB | 2 | `4Gi` / `6Gi` | 60 GB |
| 32 GB, 8 cores | about 29 GB | 5 | `4Gi` / `6Gi` | 150 GB |
| 64 GB, 16 cores | about 61 GB | 10 | `4Gi` / `8Gi` | 300 GB |

The rule behind the table: `maxSessions × memoryLimit` stays below the memory left for sandboxes, so
that no combination of sessions can exhaust the node. That costs capacity; if your sessions are
mostly practice, a lower request with the same limit fits more of them and accepts the risk. Disk is
`maxSessions × ephemeralStorageLimit`, plus 20 GB for the image cache and the system.

Nodes need cgroup v2 and should allow many inotify watches and processes:

```bash
sysctl -w fs.inotify.max_user_watches=524288 fs.inotify.max_user_instances=512
```

## Install

```bash
helm install kubedojo deploy/helm/kubedojo -n kubedojo --create-namespace -f my-values.yaml
kubectl label namespace kubedojo pod-security.kubernetes.io/enforce=privileged
```

See [`values.yaml`](../deploy/helm/kubedojo/values.yaml) for every option, and
[observability.md](observability.md) for telemetry, dashboards and alerts.

## Operations

- **Upgrades** restart the server Pod. Running sandboxes survive; sessions resume from the database.
  Open terminals reconnect with one click.
- **Shutdown.** On SIGTERM the server stops taking requests (`/api/ready` answers 503, `/api/health`
  stays 200) and waits for running operations: an exam being graded, a cluster being created. After
  `--shutdown-grace` (`KUBEDOJO_SHUTDOWN_GRACE`, default 25s) it cancels what is left. Such an exam is
  not graded with failed checks; the session shows "interrupted by a server restart". Set the Pod's
  `terminationGracePeriodSeconds` a little above the grace period, or Kubernetes kills the server
  first.
- **Backup**: the PVC holds a single SQLite file, `/data/kubedojo.db`.
- **Sandbox Pods** carry the labels `app.kubernetes.io/component=sandbox`,
  `app.kubernetes.io/instance=<release>` and `kubedojo.dev/session=<id>`:
  `kubectl -n kubedojo get pod -l app.kubernetes.io/component=sandbox`.
  - Deleting one ends that session's environment. The session then answers "the sandbox of this
    session no longer exists"; the learner ends it and starts a new one.
  - They are owned by the server's Deployment: `helm uninstall` removes them with the release.
  - At start and every five minutes the server deletes sandbox Pods that belong to no session, for
    example after it was killed while a session was being removed. A Pod younger than two minutes is
    left alone.
- **A sandbox that does not start** tells the learner why, in the session's progress log: no node
  has room (the scheduler's message), the image cannot be pulled, the Pod was evicted. It gives up
  after one minute without a node and two minutes without the image, and after
  `KUBEDOJO_SANDBOX_START_TIMEOUT` (default 8m) otherwise.
- **One replica only.** Sessions are held by the single server Pod; do not scale the Deployment.
