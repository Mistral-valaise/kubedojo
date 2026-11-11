# Observability

kubedojo exports traces, metrics and logs with OpenTelemetry over OTLP/HTTP. It is designed for the
Grafana LGTM stack (Loki, Grafana, Tempo, Mimir) behind Grafana Alloy, and works with any OTLP receiver.

## Enable it

| Mode | How |
|---|---|
| Local, batteries included | `kubedojo up --observability` — starts `grafana/otel-lgtm` and loads the dashboards. Grafana: <http://localhost:3000> |
| Local, your own collector | `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 kubedojo up` |
| Helm, your stack | `telemetry.otlpEndpoint: http://alloy.monitoring.svc:4318` |
| Helm, evaluation | `lgtm.enabled: true` — a throwaway single-container stack |

All standard `OTEL_*` environment variables are honoured (headers, per-signal endpoints, resource
attributes). Without an endpoint, telemetry is off and costs nothing.

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
| `kubedojo_sessions_active` | gauge | `mode` | live sessions; each owns a sandbox |
| `kubedojo_terminal_connections` | gauge | | open browser terminals |
| `kubedojo_sandbox_provision_duration_seconds` | histogram | `track`, `mode`, `result` | time until a cluster (practice) or a whole exam environment is ready |
| `kubedojo_check_runs_total` | counter | `track`, `question`, `mode`, `result` | graded questions |
| `kubedojo_question_score_ratio` | histogram | `track`, `mode` | share of points per graded question |
| `kubedojo_question_rating` | histogram | `track`, `question` | learner ratings, 1–5 stars (`_sum / _count` is the average) |
| `kubedojo_exams_finished_total` | counter | `track`, `result` | finished exam simulations |
| `kubedojo_exam_score_percent` | histogram | `track` | exam results |
| `http_server_request_duration_seconds` | histogram | `http_route`, `http_request_method`, `http_response_status_code` | API latency (OTel HTTP semantic conventions) |

The `question` label has one value per question (58 today). No metric carries user or session ids.

## Traces and logs

- **Traces**: one span per API request (named by route), plus `sandbox.provision` spans. The terminal
  WebSocket, health checks and static files are not traced.
- **Logs**: structured, to stderr (JSON in `serve` mode) and to OTLP, with `service.name=kubedojo`.
  Session lifecycle messages carry a `session` attribute.

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
