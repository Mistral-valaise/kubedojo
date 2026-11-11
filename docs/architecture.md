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
