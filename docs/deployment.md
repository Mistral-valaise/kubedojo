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
  read-only root filesystem. Its ServiceAccount may create, get and delete Pods in its own Namespace —
  nothing else.
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
| sandbox host | `make sandbox-host` — Docker-in-Docker with the node and terminal images preloaded |

Push both to your registry and set `image.repository`, `sandbox.image.repository` and the tags.
The sandbox host image is large (about 1.7 GB) because it carries the cluster images; preloading them
avoids pulling them for every session. Pre-pull it on the sandbox nodes for fast starts.

## Sizing

| | Memory | Notes |
|---|---|---|
| Practice session | 2–3 GB | one or two clusters, created on demand |
| Exam session | 3–5 GB | two clusters and all task setups |
| Session start | about 1 min | about 30 s to start the sandbox host and load its images, then about 35 s per cluster |

`sandbox.resources.memoryRequest` (default 3Gi) drives scheduling; `maxSessions` caps concurrency;
`idleTTL` (default 3h) removes sandboxes nobody uses. Sandbox Pods use `emptyDir` for Docker's data:
plan about 10 GB of ephemeral storage per session.

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
- **Backup**: the PVC holds a single SQLite file, `/data/kubedojo.db`.
- **Stuck sandboxes**: `kubectl -n kubedojo get pod -l app.kubernetes.io/component=sandbox`; deleting a
  sandbox Pod ends that session's environment.
- **One replica only.** Sessions are held by the single server Pod; do not scale the Deployment.
