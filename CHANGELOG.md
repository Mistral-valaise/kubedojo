# Changelog

Notable changes per release. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- **Eight more CKS questions**, 29 in total: anonymous API access, audit log forensics, container
  drift, dropping capabilities, host account lockdown, manifest static analysis, blocking the
  metadata endpoint and the kubelet's default seccomp profile. They were merged in November but
  never reached `main`. `docs/cks-coverage.md` maps every question to the curriculum.

### Changed

- The tour video has sound and is cut to the music: a music bed, a few sound effects on visible
  actions, and a new scene that solves a CKAD task with `kubectl` on a real cluster. It opens with
  the project name and ends with a QR code to the repository. Credits are in
  `docs/media/CREDITS.md`.
- **`kubedojo up --observability` is reliable.** The stack runs from a pinned `grafana/otel-lgtm`
  image (0.41.1, by digest) instead of `latest`. kubedojo waits until Grafana and the collector
  answer, and says why when they do not. The ports are flags (`--grafana-port`, `--otlp-port`), and
  an occupied port is reported with the port, the flag and its owner.
- Metrics, logs and traces are kept in a Docker volume for 7 days (`--observability-retention`) and
  survive a restart. `--observability-ephemeral` keeps nothing.
- `--observability` no longer overwrites `OTEL_EXPORTER_OTLP_ENDPOINT`. If the variable points to
  another collector, kubedojo stops with an error that names both ways out.
- New commands `kubedojo observability up | status | stop | reset`. `kubedojo down` stops the stack
  and keeps its data.
- An address that is in use (`--addr`) is reported before the sandbox checks run, not after.
- **Signals are linked in Grafana.** The local stack and the chart's `lgtm.enabled` provision their
  own data sources: from a trace to its logs and to the request metrics of its route, from a log
  line to its trace, and from a latency sample to the trace behind it (exemplars).
- **Traces show what a session did.** Preparing a question, building an exam environment, grading
  and destroying are spans below the request that started them, with the steps of creating a
  sandbox as children. Log lines written during that work carry the trace ID, and the metric
  samples carry it as an exemplar. Before, the only span besides the requests was a
  `sandbox.provision` without a parent, and no log line had a trace ID.
- **Dashboards only show what can have data.** The Pod panels moved to their own dashboard,
  *Sandbox Pods*, which only the chart installs. A local kubedojo gets *Sandbox containers*
  instead, with the memory and CPU of its Docker containers, measured by kubedojo every 30 seconds
  (`kubedojo_sandbox_*`, `kubedojo_docker_memory_limit_bytes`). New for the single-container stack:
  *Telemetry pipeline*, which shows whether telemetry arrives.
- Latency and sandbox start panels show exemplars, *Platform health* lists recent traces, and
  *Learning analytics* shows the share of points per question, a metric that was exported but never
  displayed. Panels that counted over `$__interval` use `$__rate_interval`; they lost samples.
- **Alerts in the local stack.** Seven rules are provisioned in Grafana and are evaluated from the
  start: kubedojo down, API errors, failing and slow sandbox starts, the session limit, sandbox
  memory and refused telemetry. They are visible in Grafana only; nothing is sent. The chart's
  `PrometheusRule` is rendered from the same file, `observability/rules.yaml`.
- `KubedojoDown` uses `absent_over_time`. With `absent`, a kubedojo that had stopped went unnoticed
  for five more minutes, because Prometheus keeps a pushed sample visible that long.
- `KubedojoSessionLimitReached` compares the new gauges `kubedojo_sandboxes_active` and
  `kubedojo_sandboxes_limit`. Before, it counted theory sessions, which take no slot, and could
  fire early.
- **The stack proves that it works.** `kubedojo observability status --verify` writes a trace, a log
  record and a metric and reads them back through Grafana, with the links between them, in about
  two seconds. `kubedojo up --observability` runs the same checks at start.
- The stack is stopped before it is replaced, and Tempo's store for recent traces is kept in the
  data volume. Without that, a restart lost the traces of the last minutes.
- `make lint-dashboards` checks every dashboard query against the metrics kubedojo exports.
- Helm: `lgtm.image` is pinned to the tested version, and `lgtm.enabled` no longer breaks when
  `dashboards.enabled` is off or `dashboards.namespace` is set. The stack has its own copy of the
  dashboards.
- **Helm: a sandbox has CPU and disk bounds.** New values `sandbox.resources.cpuLimit` (default 4),
  `ephemeralStorageRequest` (10Gi) and `ephemeralStorageLimit` (30Gi); the Docker volume of a
  sandbox is limited to the same size. Before, a sandbox had no CPU limit and could fill the node's
  disk. **This changes scheduling**: a node now needs 10 Gi of free ephemeral storage per sandbox.
- The server checks the sandbox resources at start and names what is wrong, and the chart has a
  schema: a misspelled key or a value like `3GB` fails `helm install` instead of a session.

### Fixed

- **Stopping the server no longer loses work.** On SIGTERM or Ctrl+C the server waits for running
  operations (an exam being graded, a cluster being created) for up to `--shutdown-grace`
  (`KUBEDOJO_SHUTDOWN_GRACE`, default 25s). An exam whose grading is cut off is marked as interrupted;
  before, it could be recorded as failed with zero points. A second Ctrl+C stops at once.
- A history entry that cannot be written is logged. Before, a failed write lost an exam result
  without a trace.
- A database migration that fails stops the server at start with the reason, instead of being
  ignored.
- The tutor request body is limited to 8 KiB like every other request.

### Added

- `GET /api/ready` for readiness probes: it fails while the server shuts down or when the database
  does not answer. `/api/health` remains the liveness check.
- `KUBEDOJO_MAX_SESSIONS`, the environment variable for `--max-sessions`.

### Security

- Built with Go 1.26.11. Releases up to 0.4.0 were built with Go 1.26.5, which has known
  vulnerabilities in `net/http`, `crypto/tls` and `html/template` that kubedojo's code reaches.
- CI now fails on known vulnerabilities in called code (`govulncheck`), runs `staticcheck` and the
  tests with the race detector, and a release is only published after these checks pass.

## [0.4.0] - 2026-11-19

The road to Kubestronaut: three new tracks, a recommended path through all six, and two kinds of
practice that need no Kubernetes cluster.

### Added

- **Linux & Vim Essentials track.** 40 exercises in 10 modules with a lesson each: navigation, files,
  text and logs, pipes, Vim (8 exercises), permissions, processes and services, networking and ssh,
  bash, incident troubleshooting. A skills track, not a certification: its assessment is 15
  exercises in 60 minutes.
- **A Linux sandbox without a cluster.** Environments of `type: container` run on one unprivileged
  Linux container with memory, CPU and process limits, on a network of its own. It is ready in about
  a second. New image `kubedojo/linux`.
- **KCNA and KCSA tracks.** 43 and 41 multiple-choice questions with a lesson per domain. Practice
  shows the explanation for every option after answering; a mock exam is 30 questions in 45 minutes
  and reveals nothing until it is finished. These tracks need no Docker.
- **Road to Kubestronaut on the home page.** The recommended order Linux & Vim → KCNA → CKAD → CKA →
  KCSA → CKS with readiness per step and how many of the five mock exams are passed. Nothing is
  locked. Defined in `content/path.yaml`.
- Track kinds (`certification`, `skills`, `theory`) in `track.yaml`. The UI words every page by kind:
  a Linux track is never presented as a Kubernetes exam.
- `docs/curriculum-design.md`: the official sources behind the tracks, the order, and the limits.
- `shellcheck` on the Linux host.
- README, home page and curriculum design say who kubedojo is for: people who build and run software
  and want to stay in practice, not only exam candidates.
- A 50-second tour of the project, `docs/media/kubedojo-promo.mp4`, linked from the README.

### Changed

- A check's `on` is validated when content is loaded. A typo used to address a machine that does
  not exist.
- `kubedojo doctor` and `content test` ask for kind and the node image only when a selected track
  uses a cluster.
- Sessions of theory tracks do not count against `--max-sessions`.
- Guard checks score only once another check of the task has passed. An untouched task used to
  earn the points of its guards in an exam.
- A quick round is no longer taken for a full assessment: its report says "quick round", and the
  path on the home page counts and ticks full mock exams, exam simulations and skills assessments
  only. Exam records carry their number of questions and minutes.
- Hands-on practice and assessments work on a phone: below the `md` breakpoint task and terminal
  are two tabs, and the terminal keeps its connection and scrollback behind the other tab.
- The Linux track is worded as a skills track throughout: recommendations, the provisioning and
  grading log, the assessment workspace and the first line of its 40 task texts.
- Counts are singular or plural ("1 exercise", "2 questions") instead of "question(s)".
- A check that waits for its retry window says so after three seconds, with the seconds it has run.
- Check explanations are rendered as Markdown; expected values of several lines keep their line
  breaks instead of showing `\n`.

### Fixed

- Lessons no longer scroll sideways on a phone.
- The solution opens without a confirmation once every check has passed.
- Long domain titles wrap in the report instead of being cut off.

## [0.3.0] - 2026-11-19

### Added

- **CKAD track.** Certified Kubernetes Application Developer: 5 domains with a lesson each,
  22 questions, full exams of 16 tasks in 120 minutes.
- `podman` and `buildah` on the exam nodes, for building and saving container images.
- Browser tests for the terminal keyboard (`make e2e`).
- `content lint` warns about checks that probe with `kubectl run --rm`.
- CI runs the content proof for the CKAD track on pull requests and every night.
- `docs/adding-a-track.md`.

### Changed

- Terminal tab shortcuts on macOS are Ctrl+Option+T/W/←/→/1–9. Other platforms keep Alt+Shift.
- On macOS the Option key types characters again instead of acting as Meta. Use `Esc` followed by
  the key for Meta shortcuts such as word movement.
- `kubedojo doctor` rebuilds a sandbox image that an older kubedojo built, so new tools reach
  existing installations.
- `kubedojo doctor` pulls the sandbox images from `ghcr.io/mistral-valaise`, where releases publish
  them, instead of trying Docker Hub.
- CI on pull requests is faster: it proves only the questions a change touches, looks once at
  checks that are expected to fail (`content test --quick`), caches the sandbox images and no
  longer waits for cross-compiling. The nightly job still proves everything in full.
- CI uses `actions/upload-artifact@v7` (Node 24).

### Fixed

- German and other keyboard layouts that type `| @ [ ] { } \ ~` with Option could not enter these
  characters in the terminal on macOS; Option+7 showed `(arg: 7)` ([#5](https://github.com/Mistral-valaise/kubedojo/issues/5)).
- CKA `fix-broken-rollout`: the "Service answers" check failed at random.
- The log line "sandboxes run as pods" in hosted mode showed an empty image when the default
  image was used.
- CKA `failing-container-logs`: the hint pointed to `logs --previous`, which usually fails while the
  container waits in CrashLoopBackOff; the worked solution could write an empty answer.

## [0.2.0] - 2026-11-11

Kubernetes 1.37, click-to-copy and run buttons, question ratings, registry pull-through cache,
Killercoda export, refreshed README.

## [0.1.0] - 2026-11-11

First release: CKA and CKS tracks with 58 verified questions, practice and exam modes, local and
Helm deployment, OpenTelemetry observability.

[Unreleased]: https://github.com/Mistral-valaise/kubedojo/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/Mistral-valaise/kubedojo/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/Mistral-valaise/kubedojo/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/Mistral-valaise/kubedojo/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/Mistral-valaise/kubedojo/releases/tag/v0.1.0
