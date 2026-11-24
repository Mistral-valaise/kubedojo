# Changelog

Notable changes per release. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [0.5.0] - 2026-11-24

An assistant that explains a failed check from your task and your cluster, with the model you
choose; a redesigned interface; traces that read from the request down to each check and each call
to the model; and a hardened local API.

**Before you upgrade**

- Behind an ingress with more than one host name, list the others in `allowedHosts` (chart) or
  `--allowed-hosts`: the server only answers for hosts it knows.
- A request with a JSON body needs `Content-Type: application/json`; without it the API answers 415.
- A local observability stack is recreated once on the next `kubedojo up --observability`; its data
  is kept.
- `KUBEDOJO_TUTOR_MODEL`, the Helm values `tutor.*` and `POST …/tutor` still work and are deprecated.

### Added

- The AI tutor becomes an **assistant**: a conversation per question instead of one answer, streamed
  while the model writes. It is described in [docs/assistant.md](docs/assistant.md). The UI for it
  follows; this release has the server side and its HTTP API.
- A second provider next to Claude: any server with the OpenAI chat-completions API, such as a local
  Ollama or LM Studio, vLLM, OpenAI or OpenRouter.
  `kubedojo up --ai --ai-provider openai --ai-base-url http://localhost:11434/v1 --ai-model llama3.2`
  runs the assistant on a local model, without a key and without anything leaving the machine.
- Flags `--ai`, `--ai-provider`, `--ai-model`, `--ai-base-url` and `--ai-trace-content` for `up` and
  `serve`, with the environment variables `KUBEDOJO_AI`, `KUBEDOJO_AI_PROVIDER`, `KUBEDOJO_AI_MODEL`,
  `KUBEDOJO_AI_BASE_URL`, `KUBEDOJO_AI_API_KEY` and `KUBEDOJO_AI_TRACE_CONTENT`. A flag wins over
  the environment, the environment over the settings file.
- On a local server the assistant can be configured while it runs: `GET` and `PUT /api/settings`,
  and `POST /api/settings/test` to try a configuration before saving it. The settings are stored in
  `settings.json` in the data directory, readable by the owner only. The API key is write-only: no
  endpoint, log line or error shows it. `kubedojo backup` does not copy the file.
- With every message the assistant sees more of the task: the hints and the solution the learner
  has opened, the learner's note, and for failing checks the output of fixed read-only commands
  (`kubectl get` and `describe` of the object a check names, the head of the file a check reads).
  The model itself runs nothing.
- The conversation of a question is stored and shown again when the question is reopened
  (`GET`, `DELETE /api/sessions/{id}/questions/{q}/assistant`). It is part of the database and of a
  backup.
- Limits for the assistant: 4000 characters per message, 10 messages per user and minute, 4 answers
  at once on the server and one per question.
- `GET /api/config` has `assistant: {enabled, configured, provider, model, writable}`.
- The base URL of a provider is checked before it is used and again when the connection is made:
  plain `http` only for loopback and private addresses, never link-local addresses (cloud metadata),
  no redirects.
- Helm: `assistant.enabled`, `assistant.provider`, `assistant.model`, `assistant.baseURL`,
  `assistant.existingSecret` and `assistant.traceContent`.
- The assistant has its place in the UI: a column next to the task and the terminal, opened with the
  **Assistant** button in the header of the practice view. On a medium screen it takes the place of
  the task while the terminal stays in view; on a phone it is a tab of its own. It is a conversation:
  answers appear while the model writes them, can be stopped and asked for again, and are there again
  when the question is reopened. See [docs/assistant.md](docs/assistant.md#in-the-ui).
- **Explain what went wrong** on a failed check opens the assistant and asks about the result as it
  stands. The review of a wrong answer to a multiple-choice question has **Explain**, and every
  question on the report of an exam has **Ask the assistant**.
- Above an answer the assistant says what kubedojo looked at for it: the task and the check results,
  the snapshot of the cluster or the host, the failing checks. Commands in an answer have the copy and
  run buttons of a task text.
- A **Settings** page for the assistant: switch, provider, base URL with presets for Ollama,
  LM Studio, OpenAI and OpenRouter, model and API key, with **Test connection** before saving. The key
  is never shown again once it is stored. Values that a flag or an environment variable sets are
  shown locked with the name of what sets them. On a hosted server the page is read-only. It is
  linked from the header of every page.
- When the assistant cannot answer it says why in a sentence and offers what helps: to try again, with
  the seconds to wait after a rate limit, or the way to the settings.
- The home page starts with one clear next step: "Continue with <track>"
  leads back to an open session or to the track you have come furthest in,
  and "Browse all tracks" jumps to the track list. The learning path sits
  next to it as a compact card with the readiness of every step.
- Every page has a footer with the version, a link to the source on GitHub
  and the author. The practice and exam views keep their full height for the
  terminal and show the same three items behind an "About kubedojo" button in
  their header.
- An address that leads nowhere shows a "Page not found" page with the way
  back, instead of the start page. A page that fails to render shows a message
  with a reload button instead of an empty window.
- Pages show placeholders in the shape of their content while they load, and
  say so when there is nothing to show.
- A trace now reads from the request down to the single check. Every check of a graded question is
  a span `check.run` below `question.check`, with its id, its command, the machine it ran on, its
  outcome (`passed`, `failed`, `timed_out`, `grader_error`), the number of attempts and the exit
  code. Retries are events of that span, not spans.
- Commands that run in a sandbox outside a check are spans `sandbox.exec` that say what they are
  for: `setup`, `cleanup`, `heal`, `solution`, `snapshot` or `diagnostic`.
- Creating a sandbox shows where the time goes: `kind.create_cluster`, `sandbox.nodes.init`,
  `sandbox.registry.configure` and `sandbox.nodes.wait_ready` for a cluster,
  `sandbox.container.start` and `sandbox.container.wait_ssh` for a Linux host, and in hosted mode
  `sandbox.pod.create` and `sandbox.pod.wait` with an event for every change of what the Pod waits
  for.
- `exam.grade` carries the score, the percentage and the result. The second look at answers that
  were still settling is one span, `exam.grade.settle`, with an event per round.
- Request spans name their session, track, mode and question, also for requests that start no
  further work. Storing an answer and opening a hint or the solution are events on them.
- `session.practice.open` says whether a practice session was created or found, and
  `terminal.attach` traces the start of a browser terminal. When a terminal ends, one log line says
  how long it was open, how many bytes went through it and why it ended.
- API responses of traced requests carry the trace in a `Server-Timing` header
  (`traceparent;desc="00-<trace id>-<span id>-01"`), so a failed request can be found in Tempo from
  the browser's network tab.
- Log lines on stderr carry `trace_id` and `span_id` when they are written inside a span, in
  `serve` (JSON) and in `up` (text).
- Metrics: `kubedojo_terminal_session_duration_seconds`,
  `kubedojo_store_operation_duration_seconds` (by `operation`) and
  `kubedojo_http_requests_rejected_total` (by `reason`).
- Requests to the Kubernetes API that create or delete a sandbox Pod are client spans.
- `docs/observability.md` describes the whole span tree, every attribute, and TraceQL queries to
  start from.
- A message to the assistant is one trace, from the request to the model: `assistant.chat` for the
  answer, `assistant.context` for what kubedojo gathers first (with the `sandbox.exec` spans of the
  commands it reads), `chat <model>` for the call to the model and the HTTP request to the provider
  below it. `assistant.chat` names the conversation, the turn and how the answer ended
  (`kubedojo.assistant.result`).
- The call to the model follows the OpenTelemetry semantic conventions for generative AI
  (`open-telemetry/semantic-conventions-genai` v0.1.0, status: development): one `chat <model>`
  client span for every provider with `gen_ai.provider.name`, the requested model and the model that
  answered, the finish reason, the response id, the time to the first text and the token counts,
  cached tokens included.
- Metrics of the conventions: `gen_ai_client_inference_duration_seconds`,
  `gen_ai_client_inference_time_to_first_chunk_seconds`, the token counters
  `gen_ai_client_inference_usage_{input,output,cache_read_input,cache_write_input}_tokens_total` and
  the per-call histograms `gen_ai_client_inference_operation_{input,output}_tokens`. And kubedojo's
  own: `kubedojo_assistant_requests_total` (by `provider`, `phase`, `result`, `track`, `mode`; it
  also counts the messages kubedojo refuses before an answer starts) and
  `kubedojo_assistant_context_duration_seconds`. The samples of an answer carry its trace as an
  exemplar. No label holds a session, a user or text.
- Prompts and answers are recorded on the `chat` span only where `--ai-trace-content` allows it: by
  default for the bundled local observability stack and nowhere else. What is recorded has the
  structure the conventions define (`gen_ai.system_instructions`, `gen_ai.input.messages`,
  `gen_ai.output.messages`), is redacted like the output of a check and cut at 32 KiB per attribute.
- A model provider gets no trace context: the HTTP request to it is a span of the trace, and it is
  sent without `traceparent` and `baggage` headers.
- The `meta` event of an answer's stream carries `traceId` when the request is traced, so that a
  client can show the trace of an answer that failed.
- A Grafana dashboard, **kubedojo / Assistant**: messages by result, failures by kind, duration and
  time to the first text by provider and model with exemplars, tokens per hour and per message, the
  time to gather the context, refused messages, recent calls to the model and the log lines of the
  assistant.
- `docs/observability.md` has a section "The assistant" with the span tree, every attribute, the
  content rule, the token counts, the metrics and TraceQL queries to start from.
- **Eight more CKS questions**, 29 in total: anonymous API access, audit log forensics, container
  drift, dropping capabilities, host account lockdown, manifest static analysis, blocking the
  metadata endpoint and the kubelet's default seccomp profile. They were merged in November but
  never reached `main`. `docs/cks-coverage.md` maps every question to the curriculum.


- **Hosted mode is tested on k3s.** A script installs kubedojo on a fresh single-node k3s and takes
  it through a learner's sessions, the observability stack, backup, upgrade, rollback, uninstall
  and reinstall (`make e2e-hosted`, the workflow `hosted`). `docs/deployment-k3s.md` is the same
  walk on a real host, with two values files in the chart's `examples/`.
- **The chart is published** as `oci://ghcr.io/mistral-valaise/charts/kubedojo` with every release.
- **`kubedojo doctor --kubernetes`** checks a cluster before kubedojo is installed into it: node
  architecture, whether the sandboxes fit, storage, Pod Security, admission of a privileged Pod
  and, with `--deep`, whether the CNI enforces NetworkPolicies. **`kubedojo doctor --hosted`**, run
  in the server's Pod, starts a probe sandbox and creates a cluster in it.
- The server image verifies the checksum of the `kind` binary it downloads. The images the chart
  runs besides kubedojo's own are pinned by digest, and a release keeps the node, terminal and
  Linux images it was built with under tags that are not overwritten.
- `SECURITY.md` lists what hosted mode relies on and where each protection ends.
- **`kubedojo backup` and `kubedojo restore`.** A backup is a consistent snapshot of the progress
  database and can be taken while the server runs. Copying the database file, which the docs
  recommended, is not consistent: SQLite keeps the latest writes in a second file. `restore` checks
  the file, keeps the database it replaces, and refuses while a server runs.
- The server locks its data directory. A second server on the same directory stops with a message
  instead of sharing the database.
- Helm: `helm uninstall` leaves the volume with the history (`persistence.keepOnUninstall`).
- `GET /api/ready` for readiness probes: it fails while the server shuts down or when the database
  does not answer. `/api/health` remains the liveness check.
- `KUBEDOJO_MAX_SESSIONS`, the environment variable for `--max-sessions`.

### Changed

- An answer of the assistant can take up to 180 seconds (the tutor gave up after 90).
- The Anthropic client is configured by kubedojo alone. `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`
  and `ANTHROPIC_BASE_URL` work as before; `ANTHROPIC_CUSTOM_HEADERS` is no longer picked up.
- A base URL for Claude from `ANTHROPIC_BASE_URL` has to pass the same check as every provider URL:
  `https`, or `http` to a loopback or private address.
- Helm: the key of the assistant is passed as `KUBEDOJO_AI_API_KEY` instead of `ANTHROPIC_API_KEY`.
- With the assistant open, the task and the terminal keep a least width; just above 1100 px the
  assistant gives way first, so that the terminal is not squeezed.
- The report of an exam arranges its two columns by the room it has, not by the width of the window.
- The web interface has a refreshed look on one set of design tokens: revised
  colours for the dark and the light theme with readable contrast everywhere
  (at least 4.5:1 for text), one type scale with 12 px as its smallest size,
  larger touch targets, and one look for cards, buttons and form fields. The
  terminal stays dark in both themes.
- The fonts (Inter and JetBrains Mono) are shipped with kubedojo and served
  from its own address. No request leaves your machine for a font, and the
  interface looks the same without internet access. The terminal waits a
  moment for its font before it opens, so that its columns line up.
- Check results show a failed check highlighted with what was expected and
  what was found; hints and the solution sit in one row below them.
- Dialogs keep the keyboard focus inside while they are open and return it to
  where it was when they close.
- Inline code in task texts is still copied with a click, and is no longer a
  stop of the Tab key: a task with dozens of snippets had dozens of stops
  before its first real control. The copy and run buttons of a code block
  are always visible on a touch screen.
- A check that fails is no longer different from a check that passes as far as the span status
  goes: both are unset, and `kubedojo.check.passed` and `kubedojo.check.outcome` say how the learner
  did. Only a check that could not be evaluated is an error (`error.type=grader_error`), on
  `check.run` and on `question.check`.
- `question.check` now starts before the question is validated, so a check that is refused (the
  question is not ready) or cannot run (its sandbox is gone) is a span that says why instead of no
  span at all.
- Spans of session work that fail carry `error.type`. A request kubedojo refuses (not found,
  conflict, a limit) is no error: its span keeps its status and carries `kubedojo.refused`.
- The spans of the Docker client (one per request to the Docker daemon, named by its raw path) are
  switched off. The phase spans cover what they showed.
- Exam grading stops waiting for answers to settle as soon as the settle time is over, instead of
  up to three seconds later.
- Removing what is left of a sandbox that could not be created is limited to two minutes.
- `usage.inputTokens` in the `done` event of an answer counts everything the model read, for every
  provider: with the Claude API it now includes the tokens read from and written to the prompt
  cache, which are still reported as `cacheReadTokens` and `cacheWriteTokens` as well.
- The provider name of a Mistral endpoint is `mistral` (it was `mistral_ai`), as the OpenTelemetry
  conventions now spell it. It shows in `GET /api/config`, in the stream's `meta` event and in
  telemetry.
- **Breaking, hosted mode:** the chart tells the server which names it is reached under: the host of
  the Ingress and the names of the Service. If people reach kubedojo under another name or an IP
  address (a second DNS name, a NodePort or LoadBalancer address, a cluster domain other than
  `cluster.local`), list it in `allowedHosts`, or they get `403`. A port-forward to `localhost` works
  as before.
- **Breaking, API:** a request with a body needs `Content-Type: application/json`, or it is answered
  with `415`. The UI always sent it. Requests without a body (start, finish, prepare, grade) need no
  header.
- A response of the API to a request that is rejected for its `Host`, its origin or its
  `Content-Type` is JSON of the form `{"error": "..."}`, like every other error.
- The API limits the size of request bodies: 64 KiB to create a session, 16 KiB to change a
  question (note, flag, answer), 8 KiB to grade, rate or ask the tutor. A larger body is rejected
  with `400`. The requests of the UI are far below these limits.
- The workspace layout and the UI primitives of the web interface were
  restructured internally: one shared header for the workspace views, a pane
  layout that can hold a third pane, a stylesheet split into partials and one
  file per UI primitive. Nothing visible changes.
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
- **Hosted mode: sandbox Pods are cleaned up.** They are owned by the server's Deployment, so
  `helm uninstall` removes them; before, privileged Pods stayed behind. The server also deletes
  sandbox Pods that belong to no session, at start and every five minutes. The Role gains `list` on
  Pods and `get` on its own Deployment.
- Ending a session whose sandbox Pod is gone no longer creates a new Pod to delete it, which took
  up to eight minutes. Work on such a session answers that the sandbox no longer exists.
- A sandbox Pod that cannot start says why in the progress log (no node has room, the image cannot
  be pulled, evicted) and gives up within a minute or two instead of eight.
- **Hosted mode: the limits of a sandbox Pod now apply to its clusters.** The Docker daemon of a
  sandbox created its containers' cgroups at the root of the node, outside the Pod. The clusters of
  a session did not count against the Pod's memory and CPU limits, and `kubectl top` showed 300 MB
  for a Pod that used a gigabyte. The sandbox host now keeps them below the Pod's cgroup.
- **Helm: an ingress without a login is refused.** `ingress.enabled` without `oidc.enabled` fails
  with an explanation; `ingress.allowUnauthenticated: true` says that access is restricted some
  other way. **Breaking** for releases that publish kubedojo without OIDC.
- Helm: the Ingress no longer carries ingress-nginx annotations. If you use ingress-nginx, set the
  two timeout annotations in `ingress.annotations`; with Traefik nothing is needed.
- Helm: the NetworkPolicy of the sandboxes is scoped to the release, and
  `sandbox.networkPolicy.extraExceptCIDRs` keeps sandboxes away from more addresses, such as the
  public address of a node.
- Helm: `shutdownGraceSeconds` (default 60) sets the server's shutdown grace period and the Pod's
  termination grace period together. The readiness probe uses `/api/ready`, and the server has
  probes behind the OIDC proxy too, where it had none. New: `extraEnv`, `extraEnvFrom`,
  `oidc.cookieSecure`.
- Helm: the evaluation stack (`lgtm.enabled`) has a retention (`lgtm.retention`, 168h), an optional
  volume (`lgtm.persistence`), an optional login (`lgtm.adminSecret`) and a memory limit, and keeps
  recent traces across a restart like the local stack does. `telemetry.headersSecret` sends
  credentials to your own OTLP receiver.
- The server checks the sandbox resources at start and names what is wrong, and the chart has a
  schema: a misspelled key or a value like `3GB` fails `helm install` instead of a session.

### Deprecated

- `POST /api/sessions/{id}/questions/{q}/tutor`. It still answers `{"markdown": "..."}` and now
  sends `Deprecation: true`. Use `POST .../assistant`.
- The environment variable `KUBEDOJO_TUTOR_MODEL`. Use `KUBEDOJO_AI_MODEL` or `--ai-model`.
- The Helm values `tutor.existingSecret` and `tutor.model`. Use `assistant.existingSecret` and
  `assistant.model`.

### Removed

- The "AI tutor" box under the hints, with its single answer. The assistant replaces it.

### Fixed

- In the development build a dialog closed itself the moment it opened (React's strict mode ran its
  effect twice). Release builds were not affected.
- The headline of the check results no longer breaks into three lines in a narrow column: the points
  move below it.
- Errors that were swallowed are now shown, each with a way to try again: a
  question of a running exam or of a report that could not be loaded (it
  showed a spinner forever), a note or a flag that could not be saved during
  an exam, the progress, the learning path and the sessions on the home page,
  and a session that could not be ended there.
- The home page fetched the list of tracks twice.
- On a phone the progress of a domain on the track page no longer wraps
  awkwardly, long question titles are no longer cut off, and the terminal of
  an exam report takes a share of the screen that fits its height.
- Accessibility: a "Skip to main content" link on every view; the practice and
  exam views have a main landmark; the divider between task and terminal has
  a name and a value; the star rating is a radio group that the arrow keys
  move through; the theme button says whether the dark theme is on; the
  question buttons of an exam are named, and a flagged question is marked by
  a flag and not by colour alone; hints and tutor answers are announced when
  they appear; all motion stops when the system asks for reduced motion.
- The span of an API request had no `http.route` attribute, which the link from a trace to the
  request metrics of its route in Grafana needs.
- An exam that was being graded while the server shut down kept looking at its failed answers in a
  tight loop until the settle time was over.
- `kubedojo serve --data <directory>` failed when the directory did not exist yet: the lock was taken
  before the directory was created.
- Dashboards and alerts count the first event of a series. A counter that appeared with its first
  event (the first check of a question, the first message to the assistant) started at 1, and
  `increase()` and `rate()` did not see that first event: with few events, panels such as "Graded
  questions" showed too little. The bundled Prometheus now writes a zero at the start of a new
  series. An existing local stack is recreated with the new setting on the next
  `kubedojo up --observability`; its data is kept.
- The number keys of a multiple-choice question work from the moment the options appear. Before, a key pressed in the first instant after the question loaded was lost.
- CKA `node-notready-kubelet`: the "kubelet service is running" check passed at random while the
  kubelet was still broken. systemd reports a unit that it restarts every second as `active` for a
  moment on each attempt; the check now also asks the kubelet's health port. Preparing the question
  fails with the reason when the kubelet does not break as intended, instead of handing out a task
  that is already solved, for example after an earlier attempt that put a kubelet at the wrong path.
- **Stopping the server no longer loses work.** On SIGTERM or Ctrl+C the server waits for running
  operations (an exam being graded, a cluster being created) for up to `--shutdown-grace`
  (`KUBEDOJO_SHUTDOWN_GRACE`, default 25s). An exam whose grading is cut off is marked as interrupted;
  before, it could be recorded as failed with zero points. A second Ctrl+C stops at once.
- A history entry that cannot be written is logged. Before, a failed write lost an exam result
  without a trace.
- A database migration that fails stops the server at start with the reason, instead of being
  ignored.
- The tutor request body is limited to 8 KiB like every other request.

### Security

- An answer of a model never loads an image: the alternative text is shown in its place. The address
  of an image could carry data out of the page; the Content-Security-Policy blocks such a request as
  well.
- The server answers only requests whose `Host` header is a name it is known by: `localhost`,
  `127.0.0.1`, `[::1]`, names below `.localhost`, the address it listens on and the host of
  `--dev-origin`. Other names get `403`. A web page could reach a local kubedojo through a domain of
  its own that resolves to `127.0.0.1` (DNS rebinding), and with it the terminal of a sandbox and the
  tutor. More names are added with `--allowed-hosts` / `KUBEDOJO_ALLOWED_HOSTS` (comma-separated,
  `*.example.com` for subdomains, `*` to turn the check off); the chart has `allowedHosts`.
  `/api/health` and `/api/ready` are answered under any name. A server that listens on every interface
  without `--allowed-hosts` accepts every name and warns at start.
- Requests that change something are rejected with `403` when they come from another origin
  (`Sec-Fetch-Site`, or `Origin` against `Host` for browsers that do not send it).
- A request body is read only when it is declared as JSON. A form on any web page can post
  `text/plain` to a local server; such a body is now answered with `415`.
- Every response carries a Content-Security-Policy (scripts, styles, images and fonts from the server
  only, no framing) and `X-Content-Type-Options`, `Referrer-Policy`, `X-Frame-Options`,
  `Cross-Origin-Opener-Policy` and `Cross-Origin-Resource-Policy`.
- Session IDs come from the system's random source and have 12 characters instead of 8. Sessions that
  exist keep their ID.
- The data directory is created with mode `0700`, and the database, its backups, the database a
  restore keeps and the settings with `0600`. What an older release created is changed when the
  server starts. `<data>/observability` is left alone: Grafana reads it from its container.
- The ssh key of a session and the kubeconfigs of its clusters reach the terminal container on the
  standard input of `docker exec`, no longer as environment arguments that `ps` shows.
- A client has 15 seconds to send a request body, and an unused connection is closed after two
  minutes. Requests that take long (grading, the terminal) are not limited.
- A panic in a request is answered with `500` and logged with its stack; before, the connection was
  closed without an answer.
- **Releases are signed.** `checksums.txt`, every image and the Helm chart carry a keyless
  [Sigstore](https://www.sigstore.dev/) signature made by the release workflow, the images and the
  chart by digest. `SECURITY.md` has the `cosign` commands under "Verifying a release".
- **Releases come with SBOMs.** Every archive has an SPDX SBOM next to it
  (`<archive>.sbom.json`, listed in `checksums.txt`). The server, node, terminal and linux images
  carry an SBOM and build provenance as attestations.
- CodeQL analyses the Go and the TypeScript code on every pull request, on `main` and weekly.
- Trivy scans the dependencies, the Dockerfiles, the chart and the server image on pull requests
  that touch them, on `main` and weekly. A HIGH or CRITICAL vulnerability that has a fix fails the
  run. Exceptions are listed with a reason and an end date in `.github/trivyignore.yaml`.
- Every GitHub Action is pinned to a commit, and Dependabot keeps the pins current. A workflow's
  token can only read; the release jobs get the one or two write permissions they need.
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

[Unreleased]: https://github.com/Mistral-valaise/kubedojo/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/Mistral-valaise/kubedojo/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/Mistral-valaise/kubedojo/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/Mistral-valaise/kubedojo/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/Mistral-valaise/kubedojo/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/Mistral-valaise/kubedojo/releases/tag/v0.1.0
