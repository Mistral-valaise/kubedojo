# Deploying kubedojo on Kubernetes

Hosted mode is meant for **one team that trusts each other** (single-tenant). Read the security section
before exposing it.

Starting from an empty machine? [deployment-k3s.md](deployment-k3s.md) is a guide to copy and paste
for a single k3s host, the setup this project tests. This document is the reference.

## What is tested, and what is not

| | |
|---|---|
| Single-node k3s (Traefik, local-path, kube-router) on amd64, in Docker | tested on every change to the chart and nightly, by [`test/e2e/run.sh`](../test/e2e/run.sh) |
| Other Kubernetes distributions | not tested. The requirements are below; `kubedojo doctor --kubernetes --deep` checks most of them |
| OpenShift | not supported as it comes: sandbox Pods need the `privileged` SCC from a cluster administrator. Untried |

What the chart needs from a cluster:

- **Privileged Pods** with a `hostPath` (`/lib/modules`, read-only) in kubedojo's Namespace.
- **cgroup v2** on the nodes, or a sandbox's limits do not cover its clusters.
- **A CNI that enforces NetworkPolicies.** The Docker daemon of a sandbox has no authentication.
- **amd64 nodes** for the published sandbox host image.
- **A default StorageClass** with `ReadWriteOnce` volumes, for the history and the image cache.
- Raised inotify limits on the nodes (see *Sizing*).

**What 0.4.0 already had, and what came after.** The chart of 0.4.0 already had the server, the sandbox
Pods, the image cache, the OIDC proxy, the NetworkPolicy, the dashboards and the alert rules. None
of it had been run on a cluster by this project's CI. Since then, tested on k3s: installation,
sandboxes with clusters, the ingress with long-lived terminals, the NetworkPolicy, limits that cover
the clusters, cleanup of sandbox Pods, backup, upgrade, rollback and uninstall. New: the preflight,
the bounds on CPU and disk, the schema, the evaluation stack's settings, and the published chart.
Still untested: OIDC against a real provider, TLS, and anything but k3s.

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

2. **NetworkPolicy** (on by default, needs a CNI that enforces it): only the server of the same
   release can reach a sandbox's Docker port, and sandboxes cannot reach private address ranges —
   your cluster's Pods, Services and, where it has a private address, the API server. Internet
   egress stays open for image pulls.

   - **The Docker port has no authentication.** The daemon of a sandbox listens on TCP 2375 without
     TLS. The NetworkPolicy is what keeps other Pods away from it. What it does not cover: a
     process with host networking on the same node, and a cluster whose CNI does not enforce
     policies. k3s enforces them unless it was started with `--disable-network-policy`. Mutual TLS
     for this port is planned, not done.
   - **A node with a public address is not in the private ranges.** On a VPS, sandboxes could reach
     the node's kubelet, API server and SSH. List such addresses:

     ```yaml
     sandbox:
       networkPolicy:
         extraExceptCIDRs: [203.0.113.10/32]
     ```
   - IPv6 egress is not allowed by the policy.

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

   The chart refuses `ingress.enabled` without `oidc.enabled`. If access is restricted some other
   way, a VPN or an authenticating proxy in front, say so with `ingress.allowUnauthenticated: true`.

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
- **The limits cover the clusters inside.** The sandbox host puts the containers of its Docker
  daemon below the Pod's own cgroup. `kubectl top pod` therefore shows what a session really uses,
  about 1 GB for one idle two-node cluster. This needs cgroup v2 on the node; on cgroup v1 the Pod
  logs a warning and only the Docker daemon itself is limited.
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

## Ingress

The chart creates a plain `Ingress` and sets no annotations: they are specific to the controller. The
terminal is a WebSocket that stays open for as long as a learner works. The server sends a ping every
25 seconds, so a proxy that counts pings as traffic never sees the connection idle.

| Controller | What it needs |
|---|---|
| **Traefik** (the default of k3s) | Nothing. Tested with k3s v1.35: a terminal that was idle for 95 seconds kept working. |
| **ingress-nginx** | `nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"` and `proxy-send-timeout: "3600"` in `ingress.annotations`. Not tested by this project's CI. |

```yaml
ingress:
  enabled: true
  host: kubedojo.example.com
  tls: {enabled: true, secretName: kubedojo-tls}
```

**The server only answers for hosts it knows.** A request whose `Host` header names anything else
gets `403`; that is what keeps a web page from reaching kubedojo under a name of its own. The chart
tells the server its `ingress.host` and the names of its Service. Everything else it is reached
under goes into `allowedHosts`: a second host name of the ingress, the address of a NodePort or
LoadBalancer, a cluster domain that is not `cluster.local`.

```yaml
allowedHosts: [dojo.example.com, "*.dojo.example.com", 192.0.2.10]
```

The health and readiness probes are exempt, because the kubelet calls the Pod by its IP.

### The assistant

The assistant is off unless you switch it on. In hosted mode it is configured here, not in the UI,
whose Settings page is read-only:

```yaml
assistant:
  enabled: true
  provider: openai              # anthropic | openai (any OpenAI-compatible server)
  baseURL: http://ollama.ai.svc.cluster.local:11434/v1
  model: llama3.2
  existingSecret: kubedojo-ai   # key "api-key"; not needed for a server without keys
```

What is sent to the model, the limits and every setting are in [assistant.md](assistant.md). A
model server on plain `http` must have a private (RFC 1918) or loopback address. Prompts and answers
are not written to traces in hosted mode unless `assistant.traceContent: "on"`.

## Check the cluster first

Two checks, one before the installation and one after. Neither changes anything that stays.

**Before**, from your machine, with `kubectl` and the kubedojo binary:

```bash
kubedojo doctor --kubernetes --max-sessions 5 --deep
```

```text
  ✓ the cluster answers: Kubernetes v1.35.5+k3s1
  ✓ node mini: amd64, kernel 6.8.0, containerd://2.2.3-k3s1, kubelet v1.35.5+k3s1
  ✓ node mini has room for 8 sandbox(es): 7.3 CPU, 29.1 Gi memory, 180 Gi disk free of requests; CPU runs out first
  ✓ room for 8 sandboxes, 5 wanted (each requests 1 CPU, 3Gi memory, 10Gi disk)
  ✓ default StorageClass local-path
  ✓ ingress controller(s): traefik
  ✓ the Namespace kubedojo admits privileged Pods
  ✓ the API server admits a privileged Pod with /lib/modules in kubedojo (dry run)
  ✓ the CNI enforces NetworkPolicies: an isolated Pod could not be reached
```

| It checks | It fails when |
|---|---|
| the nodes: architecture, kernel, runtime | a node is not `amd64`, the architecture of the published sandbox host image (`--sandbox-arch any` if you built your own) |
| free CPU, memory and disk against what the sandboxes request | fewer sandboxes fit than `--max-sessions`; it names the resource that runs out first |
| a default StorageClass | there is none |
| the Pod Security level of the Namespace | it is `baseline` or `restricted` |
| admission of a privileged Pod with a `hostPath` | the API server refuses it in a server-side dry run |
| ingress controllers | never; it lists them |
| node addresses | never; it warns about public ones and prints the `extraExceptCIDRs` to set |
| with `--deep`: NetworkPolicy enforcement | a Pod that a policy isolates can be reached. Then every sandbox's Docker port would be open to the cluster |

`--deep` starts two small Pods and a policy in the Namespace and removes them. The other checks only
read. Use `--context` and `--namespace` for another cluster or Namespace; the sandbox requests are
read from the same `KUBEDOJO_SANDBOX_*` variables the server uses.

**After**, in the server's Pod:

```bash
kubectl -n kubedojo exec deploy/kubedojo -c kubedojo -- kubedojo doctor --hosted --cluster
```

```text
  ✓ the sandbox Pod is ready after 33s
  ✓ Docker in the sandbox answers: 29.10.0 cgroup 2
  ✓ a cluster was created in the sandbox in 27s
```

It uses the server's own configuration: the image, the resources, the node selector. A Pod that does
not start reports why, and a Docker that does not answer means the server cannot reach the sandbox,
which is a NetworkPolicy or the CNI. The probe Pod is removed afterwards.

## Install

The chart is published with every release, next to the images it installs:

```bash
kubectl create namespace kubedojo
kubectl label namespace kubedojo pod-security.kubernetes.io/enforce=privileged
helm install kubedojo oci://ghcr.io/mistral-valaise/charts/kubedojo --version <release> \
  -n kubedojo -f my-values.yaml --wait --timeout 10m
```

The label comes first: without it the first sandbox is refused. From a checkout,
`deploy/helm/kubedojo` takes the place of the `oci://` address.

Two complete values files to start from are in
[`examples/`](../deploy/helm/kubedojo/examples): a private host without a login, and a public one
with HTTPS and OIDC. See [`values.yaml`](../deploy/helm/kubedojo/values.yaml) for every option, and
[observability.md](observability.md) for telemetry, dashboards and alerts.

All images are pinned: kubedojo's own to the release, the others (`registry`, `oauth2-proxy`,
`grafana/otel-lgtm`, the Docker-in-Docker base) by digest.

## Operations

- **Upgrade with `helm upgrade --reset-then-reuse-values`**, not `--reuse-values`: the latter keeps
  the values of the old release and does not see defaults that are new in the chart. The templates
  tolerate missing new values, but you would not get new settings you might want.
- **Upgrades** restart the server Pod. Running sandboxes survive; sessions resume from the database.
  Open terminals reconnect with one click.
- **Shutdown.** On SIGTERM the server stops taking requests (`/api/ready` answers 503, `/api/health`
  stays 200) and waits for running operations: an exam being graded, a cluster being created. After
  `--shutdown-grace` (`KUBEDOJO_SHUTDOWN_GRACE`, default 25s) it cancels what is left. Such an exam is
  not graded with failed checks; the session shows "interrupted by a server restart". Set the Pod's
  `terminationGracePeriodSeconds` a little above the grace period, or Kubernetes kills the server
  first. The chart does both from one value, `shutdownGraceSeconds` (default 60): the grace period,
  and 15 seconds more for the Pod.
- **Uninstall keeps the history.** `helm uninstall` leaves the server's volume
  (`persistence.keepOnUninstall`, on by default); a release of the same name in the same Namespace
  takes it over again. To delete the history: `kubectl -n kubedojo delete pvc kubedojo`.
- **One server per data directory.** The server holds a lock on `/data`. A second one on the same
  volume, or a restore while it runs, stops with a message.

### Backup and restore

The history is one SQLite database, `/data/kubedojo.db`. Do not copy the file while the server runs:
SQLite keeps the latest writes in a second file, and a copy of the first alone is not consistent.
`kubedojo backup` takes a consistent snapshot and works under a running server:

```bash
kubectl -n kubedojo exec deploy/kubedojo -c kubedojo -- kubedojo backup --out /data/backup.db
kubectl -n kubedojo exec deploy/kubedojo -c kubedojo -- cat /data/backup.db > kubedojo-backup.db
```

It prints what the backup holds: sessions, graded questions, exams, ratings. Keep the file somewhere
that is not this volume.

To restore, the server must not run. Stop it, restore in a Pod of its own that mounts the volume, and
start it again (the image is the server's, see `kubectl -n kubedojo get deploy kubedojo -o
jsonpath='{.spec.template.spec.containers[0].image}'`):

```bash
kubectl -n kubedojo scale deploy/kubedojo --replicas=0
kubectl -n kubedojo run kubedojo-restore --rm -i --restart=Never --image=<server image> --overrides='
{"spec":{"securityContext":{"runAsUser":10001,"fsGroup":10001},
 "containers":[{"name":"kubedojo-restore","image":"<server image>",
   "args":["restore","--from","/data/backup.db"],
   "volumeMounts":[{"name":"data","mountPath":"/data"}]}],
 "volumes":[{"name":"data","persistentVolumeClaim":{"claimName":"kubedojo"}}]}}'
kubectl -n kubedojo scale deploy/kubedojo --replicas=1
```

`restore` checks that the file is an intact kubedojo database before it touches anything, and keeps
the database it replaces as `kubedojo.db.before-restore-<time>`. For a backup that is not on the
volume yet, copy it there first, into the same kind of Pod.

Sandboxes are not part of a backup. Sessions that were open when it was taken come back without
their clusters; learners end them and start new ones. The history, the exam results and the ratings
are what a backup is for.

### Upgrade and rollback

```bash
kubectl -n kubedojo exec deploy/kubedojo -c kubedojo -- kubedojo backup --out /data/before-upgrade.db
helm upgrade kubedojo <chart> -n kubedojo --reset-then-reuse-values
helm rollback kubedojo -n kubedojo        # back to the previous revision
```

- The server Pod is replaced, not rolled: there is one, and the old one stops before the new one
  starts. It waits for running work first (see *Shutdown* above).
- Sandboxes keep running and sessions go on; open terminals reconnect.
- The database is changed only by adding columns. An older server therefore still reads a database a
  newer one has used, and a rollback needs no restore. The backup is for the case that this ever
  stops being true.
- A sandbox keeps the sandbox host image it was started with. After an upgrade, old sessions run the
  old image and new sessions the new one, until the old ones end.

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
