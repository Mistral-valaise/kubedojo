# Architecture

## Packages

| Package | Responsibility |
|---|---|
| `cmd/kubedojo` | CLI: `up`, `serve`, `doctor`, `down`, `content lint/test` |
| `internal/api` | REST API, terminal WebSocket, embedded UI |
| `internal/session` | lifecycle of practice and exam sessions, grading, reports |
| `internal/sandbox` | `Driver` interface and the kind driver |
| `internal/dockerhost` | which Docker endpoint a sandbox lives on |
| `internal/k8shost` | sandbox Pods for hosted mode |
| `internal/checker` | runs checks, compares results |
| `internal/catalog` | loads and validates content |
| `internal/store` | SQLite: sessions, attempts, exam results |
| `internal/telemetry` | OpenTelemetry setup and instruments |
| `internal/tutor` | optional AI tutor (Claude API) |
| `internal/export` | export questions to other platforms (Killercoda) |
| `web/` | React + TypeScript UI, xterm.js terminal |
| `content/` | tracks, lessons, questions |

## One driver, two Docker endpoints

The sandbox code only ever talks to "a Docker endpoint" through the `docker` and `kind` CLIs and the
Engine API (for the interactive terminal). Locally that endpoint is the machine's Docker socket. In
hosted mode `internal/k8shost` starts a Docker-in-Docker Pod per session and hands its address to the
same code. Nothing above `dockerhost` knows the difference.

## A sandbox

- `kd-<session>-terminal` — the `candidate@terminal` container the browser attaches to, with kubectl,
  helm, kustomize, trivy, etcdctl and an ssh identity for the session.
- One kind cluster per environment, named like an exam host (`cka3827`). Nodes are `kindest/node`
  containers plus sshd and a `candidate` user, so `ssh cka3827` works as in the exam.

Checks, setup and solution scripts run through `docker exec` on the nodes.

### Terminals

Every terminal tab in the UI is one WebSocket (`GET /api/sessions/{id}/terminal`) and one
`docker exec bash -l` with a TTY in the terminal container. Tabs are therefore independent shells —
own process, own scrollback, own window size — that share the sandbox: same files, same kubeconfig,
same clusters. No tab creates a cluster.

- **Limit.** `session.Manager` counts the open terminals of each session and refuses more than
  `--max-terminals` (default 4). The slot is reserved before the shell starts, so parallel requests
  cannot exceed it. The server answers a refused terminal with WebSocket close code `4429`; the UI
  disables **+** at the limit.
- **Cleanup.** Closing the WebSocket — tab closed, page left, exam finished, network gone — closes the
  terminal. Closing the attach stream does not end the shell by itself, so the driver hangs up every
  process that carries the tab's `KD_TTY` marker in its environment (SIGHUP, then SIGKILL for a shell
  that stays). The container runs with an init process that reaps them. Destroying a session closes
  all of its terminals first.
- **In the browser** all tabs stay mounted; background tabs are hidden with `visibility`, keep
  receiving output and are resized together with the visible one. "Run" buttons type into the active
  tab (`web/src/terminalBus.ts`).

## Sessions

- **Practice**: one long-lived session per track and user. Clusters are created the first time a
  question needs them and reused. Each question owns its Namespaces and is reset by re-running its setup.
- **Exam**: questions are picked by domain weight, all clusters are created and all setups run before
  the timer can be started. Grading happens once, at the end or at the deadline.

State lives in memory and is persisted to SQLite on every change, so a restarted server finds its
sessions again. That is also why the server is a single replica.

## Adding a driver

Implement `sandbox.Driver` (`CreateEnv`, `EnsureTerminal`, `Exec`, `Attach`, `DeleteEnv`, `Destroy`).
A VM driver (Lima locally, KubeVirt hosted) is the planned way to support tasks that need a real kernel
boundary; questions opt in with `driver: vm`.
