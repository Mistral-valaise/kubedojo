# Observability

kubedojo exports traces, metrics and logs with OpenTelemetry over OTLP/HTTP. It is designed for the
Grafana LGTM stack (Loki, Grafana, Tempo, Mimir) behind Grafana Alloy, and works with any OTLP receiver.

## Enable it

| Mode | How |
|---|---|
| Local, batteries included | `kubedojo up --observability` — starts the [local stack](#the-local-stack) and loads the dashboards. Grafana: <http://localhost:3000> |
| Local, your own collector | `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 kubedojo up` |
| Helm, your stack | `telemetry.otlpEndpoint: http://alloy.monitoring.svc:4318` |
| Helm, evaluation | `lgtm.enabled: true` — a throwaway single-container stack |

The standard `OTEL_*` environment variables for endpoints, headers and resource attributes are
honoured. The transport is always OTLP over HTTP; `OTEL_EXPORTER_OTLP_PROTOCOL` is not read. Without
an endpoint, telemetry is off and costs nothing.

## The local stack

`kubedojo up --observability` runs one container, `kubedojo-lgtm`, from the image
`grafana/otel-lgtm`. It holds Grafana, Prometheus, Loki, Tempo and an OpenTelemetry Collector. The
image is pinned by version and digest to the one kubedojo is tested with. It needs Docker and nothing
else: no Helm, no cluster.

```sh
kubedojo up --observability        # kubedojo on :4400, Grafana on :3000
kubedojo observability status      # does it run, and where?
kubedojo observability stop        # stop it, keep the data
kubedojo observability reset       # stop it and delete the data
```

| Flag | Environment variable | Default | |
|---|---|---|---|
| `--observability` | `KUBEDOJO_OBSERVABILITY` | off | start the stack and send telemetry to it |
| `--grafana-port` | `KUBEDOJO_GRAFANA_PORT` | 3000 | local port of Grafana |
| `--otlp-port` | `KUBEDOJO_OTLP_PORT` | 4318 | local port of the OTLP/HTTP receiver |
| `--observability-retention` | `KUBEDOJO_OBSERVABILITY_RETENTION` | 168h | how long metrics, logs and traces are kept; at least 24h |
| `--observability-ephemeral` | `KUBEDOJO_OBSERVABILITY_EPHEMERAL` | off | keep no data volume |
| `--observability-image` | `KUBEDOJO_OBSERVABILITY_IMAGE` | the pinned image | run another `grafana/otel-lgtm` |

What to expect:

- **Loopback only.** Grafana and the OTLP receiver listen on 127.0.0.1. Grafana has no login; do not
  publish its port.
- **Starting twice is safe.** A running stack with the same settings is kept. With other settings it
  is replaced; the data stays. If the new ports are taken, the running stack is left alone and the
  error names the port, the flag and, if it is a container, who holds it.
- **Your own collector wins.** If an `OTEL_EXPORTER_OTLP_*ENDPOINT` variable points somewhere else,
  `--observability` stops with an error instead of redirecting your telemetry.
- **Data.** Metrics, logs and traces live in the Docker volume `kubedojo-lgtm-data` and survive
  `stop`, `kubedojo down` and a new kubedojo version. `kubedojo observability reset` deletes them.
  Your progress is in `~/.kubedojo/kubedojo.db` and is never touched by any of this.
- **Size.** About 550 MB of memory when idle and about 700 MB once telemetry arrives, measured on
  macOS arm64 with Docker Desktop. The image is 0.9 GB to download. The volume starts at about 450 MB,
  most of it Grafana plugins the image installs on first start.
- **Only kubedojo's container.** A container named `kubedojo-lgtm` that kubedojo did not create is
  never stopped or removed; the commands say so and stop.

A minimal Alloy pipeline:

```alloy
otelcol.receiver.otlp "default" {
  http { endpoint = "0.0.0.0:4318" }
  output {
    metrics = [otelcol.exporter.prometheus.mimir.input]
    logs    = [otelcol.exporter.loki.default.input]
    traces  = [otelcol.exporter.otlp.tempo.input]
  }
}
```

## Metrics

Names as they appear in Prometheus/Mimir:

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `kubedojo_sessions_active` | gauge | `mode` | live sessions; each owns a sandbox, except in the theory tracks |
| `kubedojo_terminal_connections` | gauge | | open browser terminals |
| `kubedojo_sandbox_provision_duration_seconds` | histogram | `track`, `mode`, `result` | time until a cluster (practice) or a whole exam environment is ready |
| `kubedojo_check_runs_total` | counter | `track`, `question`, `mode`, `result` | graded questions |
| `kubedojo_question_score_ratio` | histogram | `track`, `mode` | share of points per graded question |
| `kubedojo_question_rating` | histogram | `track`, `question` | learner ratings, 1–5 stars (`_sum / _count` is the average) |
| `kubedojo_exams_finished_total` | counter | `track`, `result` | finished exam simulations |
| `kubedojo_exam_score_percent` | histogram | `track` | exam results |
| `http_server_request_duration_seconds` | histogram | `http_route`, `http_request_method`, `http_response_status_code` | API latency (OTel HTTP semantic conventions) |

The `question` label has one value per question (212 today). No metric carries user or session ids.

## Traces and logs

- **Traces**: one span per API request (named by route). The terminal WebSocket, health checks and
  static files are not traced. Work on a session is traced below the request that started it, also
  when it goes on after the request was answered:

  | Span | Children | When |
  |---|---|---|
  | `question.prepare` | `sandbox.provision`, `question.setup` | a practice question is prepared |
  | `exam.provision` | `sandbox.provision`, `question.setup` per question | an exam environment is built |
  | `sandbox.provision` | `sandbox.host.acquire`, `sandbox.env.create`, `sandbox.terminal.connect` | a cluster or a Linux host is created |
  | `question.check` | | a question is graded |
  | `exam.grade` | `question.check` per question | an exam is graded |
  | `sandbox.destroy` | | a sandbox is removed |

  Spans carry `kubedojo.session.id`, `kubedojo.track`, `kubedojo.mode` and, where it applies,
  `kubedojo.question` and `kubedojo.environment`. Work the server starts itself (an exam whose time
  is up, an idle sandbox) is a trace of its own.
- **Logs**: structured, to stderr (JSON in `serve` mode) and to OTLP, with `service.name=kubedojo`.
  Session lifecycle messages carry a `session` attribute, and every line written inside a span
  carries its trace and span ID.

### How the signals are linked

| From | To | By |
|---|---|---|
| a metric sample | the trace that produced it | exemplars on `http_server_request_duration_seconds`, `kubedojo_sandbox_provision_duration_seconds`, `kubedojo_check_runs_total` and the score histograms |
| a trace | its log lines | the trace ID on the log line |
| a log line | its trace | the same |
| a trace | request rate, error rate and latency of its route | the `service.name` and `http.route` of the span |

The links are real identifiers, never matching timestamps. Session and user never become metric
labels: they exist on spans and log lines only.

## Dashboards

In [`deploy/helm/kubedojo/dashboards`](../deploy/helm/kubedojo/dashboards), installed by the chart as
ConfigMaps labelled `grafana_dashboard: "1"` for the Grafana sidecar:

| Dashboard | Answers |
|---|---|
| **Platform health** | Is it up, fast and error free? Request rate, errors, latency, slow routes, warning and error logs |
| **Sessions & sandboxes** | Do we have capacity? Active sessions, start times and failures, memory and CPU per sandbox Pod |
| **Learning analytics** | What do people practise and where do they struggle? Pass rates, hardest and lowest rated questions, exam results |

The sandbox Pod panels need cAdvisor and kube-state-metrics in the same data source.

## Alerts

`alerts.enabled: true` installs a `PrometheusRule` (Prometheus Operator, or Mimir through Alloy's
`mimir.rules.kubernetes`):

| Alert | Fires when |
|---|---|
| `KubedojoDown` | no metrics for 10 minutes |
| `KubedojoSandboxProvisioningFails` | more than 20% of sandbox starts fail |
| `KubedojoSandboxProvisioningSlow` | p95 start time above 5 minutes |
| `KubedojoSessionLimitReached` | all session slots in use for 15 minutes |
| `KubedojoHighErrorRate` | more than 5% of API requests return 5xx |

## Learner feedback

After grading, learners can rate a question with one to five stars and a comment. Averages appear on the
Learning analytics dashboard; the comments stay in the database and are printed, worst questions first, by:

```bash
kubedojo ratings            # add --track cka to filter
```
