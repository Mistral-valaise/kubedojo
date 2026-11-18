# Architecture

## Packages

| Package | Responsibility |
|---|---|
| `cmd/kubedojo` | CLI: `up`, `serve`, `doctor`, `down`, `ratings`, `content lint/test/export` |
| `internal/api` | REST API, terminal WebSocket, embedded UI |
| `internal/session` | lifecycle of practice and exam sessions, grading, reports |
| `internal/sandbox` | `Driver` interface and the kind driver: clusters and Linux hosts |
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
  helm, kustomize, trivy and an ssh identity for the session.
- One environment per name in the track's `environments`, named like an exam host (`cka3827`). What
  it is depends on its `type`:
  - `cluster` (the default): a kind cluster. Nodes are `kindest/node` containers plus sshd and a
    `candidate` user, so `ssh cka3827` works as in the exam. They also carry what tasks need on the
    host: etcdctl, helm, kustomize, trivy, podman and buildah.
  - `container`: one plain Linux container, see [Container environments](#container-environments).

Checks, setup and solution scripts run through `docker exec` on the nodes or the Linux host. Which
machine and which account follows from the environment (`sandbox.EnvSpec`: `DefaultRole`,
`ScriptUser`, `SolveUser`); the session code names no machine.

### Container environments

A track that teaches the shell needs a Linux host, not a cluster. Its environment has
`type: container`, and the sandbox is two containers on one network:

```text
kd-<session>-terminal  --ssh-->  linux4821          both on the bridge kd-<session>
  candidate@terminal             candidate@linux4821 (image kubedojo/linux:trixie)
```

The terminal stays the jump host, so the learner types `ssh linux4821` exactly as in the other
tracks. No `kind` binary, no node image and no `kind` Docker network are involved: the sandbox is up
in about a second, and `kubedojo doctor --track <id>` and `content test --track <id>` ask for kind
only when a selected track has a cluster (`Track.NeedsCluster`), for the linux image only when one
has a container environment (`Track.NeedsContainer`). A track uses one type; the loader rejects a mix.

- **The image** (`images/linux`) is Debian with man pages, the usual text and network tools, Vim,
  sshd and a `candidate` user with passwordless sudo. There is **no systemd**: PID 1 is Docker's
  init, the entrypoint installs the session's public key (`KD_PUBKEY`), generates host keys, starts
  sshd as a daemon and sleeps. Because the entrypoint does this on every start, ssh works again
  after the container or Docker restarts. Exercises about services use real processes,
  `service`/`/etc/init.d` and the `kd-fixture` helper (see [writing-questions.md](writing-questions.md)).
- **Isolation.** The learner is root in the lab (through sudo), so the container is the boundary:
  not privileged, Docker's default capabilities and seccomp profile, no host mounts, no Docker
  socket, no host PID or network namespace. Each session has its own bridge network, so labs of
  different sessions cannot reach each other; the lab can reach the internet like any container.
  This is the isolation of a container, not of a VM: the boundary the terminal container has always
  had. (The kind nodes of the cluster tracks are privileged containers, a weaker boundary than this.)
- **Limits.** `--memory 1g --memory-swap 1g --pids-limit 512 --cpus 2` (`sandbox.labRunArgs`, pinned
  by a unit test). A fork bomb or a runaway process hits the limit of its own container. Disk is not
  limited: an exercise or a learner can fill the Docker host's disk, as on a kind node.
- **Lifecycle.** The lab is created when the first question of the environment is prepared and
  reused by all others: in an exam every setup of the track runs in the one container, which is why
  each exercise keeps to its own directory, users and ports. `freshEnvironment` questions and
  `dropEnv` remove the container and start a new one under a new name. Nothing is persisted outside
  the container: no volumes.
- **Cleanup.** The lab, the terminal and the network carry the label `kubedojo.session=<id>`.
  `Destroy` removes the containers with that label and then the network; `kubedojo down` does the
  same for every session. The idle TTL (`--idle-ttl`) applies as for clusters.
- **Concurrency.** A lab costs a few MB when idle and at most 1 GB, so `--max-sessions` can be much
  higher for a host that serves only such tracks than for clusters. Two things bound it: every
  session takes one Docker bridge network, and Docker's default address pools hold about 30 of them
  (raise `default-address-pools` in the daemon configuration for more), and the name of a lab is
  drawn like a cluster name, from 9000 per track id on one Docker endpoint.
- **Hosted mode.** Nothing changes above `dockerhost`: the session's Docker-in-Docker Pod runs the
  two containers instead of a kind cluster, and the sandbox-host image carries the linux image as a
  third tarball. The Pod is privileged, as Docker-in-Docker requires. A hosted
  installation that serves only container tracks does not need that Pod per session at all — a
  driver that starts the lab as a Pod of its own would be cheaper and better isolated; it does not
  exist yet. Until then the memory request of the sandbox Pod, sized for a cluster, is far more than
  a lab uses.

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

### Keyboard layouts

Keys travel browser → xterm.js → WebSocket (binary frame, UTF-8) → `docker exec` TTY → shell. The
server and the terminal image do not translate input, so what a key types is decided in the browser,
by two things in `web/src/components/Terminal.tsx`:

- **The Option key on macOS types characters.** Many layouts need it for shell symbols: on a German
  keyboard Option+7 is `|`, Option+5/6 `[ ]`, Option+8/9 `{ }`, Option+L `@`, Option+N `~` and
  Option+Shift+7 `\`. xterm.js is therefore left at its default, `macOptionIsMeta: false`. With
  `true` it sends `ESC` plus the key's US meaning instead — Option+7 became `ESC 7`, which readline
  shows as `(arg: 7)` and Vim reads as a command ([#5](https://github.com/Mistral-valaise/kubedojo/issues/5)).
  The price: Option+B/F no longer move by word on macOS. Press `Esc` and then the key instead, as in
  any terminal without a Meta key.
- **Tab shortcuts use modifiers that type nothing.** `shortcutOf` matches T, W, the arrows and 1–9 on
  the physical key (`code`) with Ctrl+Option on macOS and Alt+Shift elsewhere. Alt+Shift on macOS
  would swallow Option+Shift characters (German `\`); Ctrl+Alt elsewhere is how Windows reports AltGr.
  A key pressed with AltGr is never a shortcut.

Covered layouts: German and US on macOS, German on Windows and Linux (AltGr). Other layouts that
type symbols with Option or AltGr work for the same reason.

**Tests.** `Terminal.test.ts` checks the shortcut mapping against the events these layouts produce.
`web/e2e/keyboard.spec.ts` (`make e2e`) sends those events through the real xterm.js in Chromium and
asserts the bytes on the WebSocket. Neither can switch the operating system's layout: they replay the
event a layout produces (`key: "|"`, `code: "Digit7"`, `altKey`), not the physical key. So after
changing anything here, check by hand on a Mac with a German layout:

1. Open any question, click into the terminal.
2. `printf 'ok\n' | cat` prints `ok`; no `(arg: 7)` appears while typing.
3. `vim /tmp/t`, press `i`, type `| [ ] { } @ \ ~` — the characters appear in the buffer.
4. Ctrl+C, Ctrl+R, the arrow keys and paste (Cmd+V) behave as before.
5. Ctrl+Option+T opens a tab, Ctrl+Option+1/2 and Ctrl+Option+←/→ switch, Ctrl+Option+W closes.

## Sessions

- **Practice**: one long-lived session per track and user. Environments (clusters or Linux hosts) are
  created the first time a question needs them and reused. Each question owns its Namespaces and is reset by re-running its setup.
- **Exam**: questions are picked by domain weight, all clusters are created and all setups run before
  the timer can be started. Grading happens once, at the end or at the deadline.

State lives in memory and is persisted to SQLite on every change, so a restarted server finds its
sessions again. That is also why the server is a single replica.

## Adding a driver

Implement `sandbox.Driver` (`CreateEnv`, `EnsureTerminal`, `Exec`, `Attach`, `DeleteEnv`, `Destroy`).
A driver is told the type of every environment (`EnvSpec.Type`, `Env.Type`) and decides what a
cluster and what a Linux host is on its platform.
A VM driver (Lima locally, KubeVirt hosted) is the planned way to support tasks that need a real kernel
boundary; questions opt in with `driver: vm`.
