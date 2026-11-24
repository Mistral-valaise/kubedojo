### Added

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

### Changed

- A check that fails is no longer different from a check that passes as far as the span status
  goes: both are unset, and `kubedojo.check.passed` and `kubedojo.check.outcome` say how the learner
  did. Only a check that could not be evaluated is an error (`error.type=grader_error`), on
  `check.run` and on `question.check`.
- `question.check` now starts before the question is validated, so a check that is refused (the
  question is not ready, its sandbox is gone) is a span with the reason in `error.type` instead of
  no span at all.
- Spans of session work that fail carry `error.type`.
- The spans of the Docker client (one per request to the Docker daemon, named by its raw path) are
  switched off. The phase spans cover what they showed.
- Exam grading stops waiting for answers to settle as soon as the settle time is over, instead of
  up to three seconds later.
- Removing what is left of a sandbox that could not be created is limited to two minutes.

### Fixed

- The span of an API request had no `http.route` attribute, which the link from a trace to the
  request metrics of its route in Grafana needs.
- An exam that was being graded while the server shut down kept looking at its failed answers in a
  tight loop until the settle time was over.
