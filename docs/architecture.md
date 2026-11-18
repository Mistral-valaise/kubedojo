# Architecture

## Packages

| Package | Responsibility |
|---|---|
| `cmd/kubedojo` | CLI: `up`, `serve`, `doctor`, `down`, `ratings`, `content lint/test/export` |
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
  helm, kustomize, trivy and an ssh identity for the session.
- One kind cluster per environment, named like an exam host (`cka3827`). Nodes are `kindest/node`
  containers plus sshd and a `candidate` user, so `ssh cka3827` works as in the exam. They also
  carry what tasks need on the host: etcdctl, helm, kustomize, trivy, podman and buildah.

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

- **Practice**: one long-lived session per track and user. Clusters are created the first time a
  question needs them and reused. Each question owns its Namespaces and is reset by re-running its setup.
- **Exam**: questions are picked by domain weight, all clusters are created and all setups run before
  the timer can be started. Grading happens once, at the end or at the deadline.

State lives in memory and is persisted to SQLite on every change, so a restarted server finds its
sessions again. That is also why the server is a single replica.

### Sessions without a sandbox

The questions of a theory track (`kind: theory`, `Track.Sandboxless()`) are multiple choice. Their
sessions use the same `Session` type, the same routes and the same store, but nothing is provisioned
(`internal/session/quiz.go`):

- They never acquire a host and never call the driver, not even on `Destroy`: in hosted mode acquiring
  a host creates a Pod. They have no key pair and no terminal.
- They do not count against `--max-sessions`, which limits sandboxes.
- `Prepare` and `Finish` are synchronous. Preparing draws the order of the options; finishing an exam
  grades it inline, with no settle time, and the report exists when the call returns.
- An answer is graded into the same `checker.Result` a task produces (one point or none, no checks), so
  attempts, exam results, progress and telemetry need no second path. Readiness counts the *latest*
  attempt at a choice question, not the best one: having clicked the right option once after seeing the
  answer says little.

**Where the answer key lives.** Only in the catalog: `Question.Choice` (correct options, explanations,
references) is tagged `json:"-"`. A `Session` holds what the learner did and saw, `QState.Order` (the
option ids in the order drawn) and `QState.Answer` (the ids picked), because a session is serialised as
it is into SQLite and into API responses. Grading looks the key up in the catalog at that moment.

The API adds the key to a question as `choice.review` only when `Session.Revealed` says so: in practice
once the question was graded or its solution was opened (it can then be answered again only after a
reset, which also reshuffles), in an exam once the exam is finished. Until then `Answer` is the only
call that takes option ids, and no response says which are correct. Option ids are a hash of the option
text, so they say nothing about position or correctness.

## Adding a driver

Implement `sandbox.Driver` (`CreateEnv`, `EnsureTerminal`, `Exec`, `Attach`, `DeleteEnv`, `Destroy`).
A VM driver (Lima locally, KubeVirt hosted) is the planned way to support tasks that need a real kernel
boundary; questions opt in with `driver: vm`.
