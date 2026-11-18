# Cloud Native Architecture

**12% of the exam.** The view from above: how you know what a system is doing, the building blocks
around Kubernetes and the standards between them, and how the community behind all of it is
organised.

## The 20% that solves 80%

### Observability

Observability is the ability to understand the inner state of a system from its outputs. The outputs
are called **signals** or telemetry.

| Signal | What it is | Answers |
|---|---|---|
| Metrics | numeric measurements over time | how much, how often, how fast? Is something wrong? |
| Logs | timestamped records of events | what exactly happened in this component? |
| Traces | the path of one request through all services, built from **spans** | where did this request spend its time or fail? |

- **Prometheus** is the metrics system of the ecosystem. The server **pulls**: it scrapes an HTTP
  endpoint that each target exposes, and stores the samples as time series, identified by a metric
  name and labels. PromQL queries them. Short-lived jobs push to a **Pushgateway**, which is scraped
  in turn; alerts go to the **Alertmanager**; Grafana is the usual place for dashboards.
- **OpenTelemetry** is a vendor-neutral framework to generate, collect and export traces, metrics and
  logs: APIs and SDKs for instrumentation, the OTLP protocol, and the **Collector** that receives,
  processes and exports telemetry. It is **not a backend**: storing and displaying the data is left
  to other tools such as Prometheus or Jaeger.
- In Kubernetes a container logs to stdout and stderr, the node keeps the files, `kubectl logs` reads
  them. Logs that must survive Pods and nodes need a cluster-level logging solution.

| Term | Is | Example |
|---|---|---|
| SLI, indicator | a measurement of service quality | share of successful requests |
| SLO, objective | the target for an SLI over a period | 99.9% over 30 days |
| SLA, agreement | a contract with consequences if the level is missed | credit below 99.5% |
| Error budget | 100% minus the SLO | 0.1% of requests may fail |

### Autoscaling

| Autoscaler | Changes | Reacts to |
|---|---|---|
| HorizontalPodAutoscaler (HPA) | the number of Pods of a workload | CPU, memory or other metrics |
| VerticalPodAutoscaler (VPA) | the resource requests of Pods | observed usage |
| Cluster Autoscaler, Karpenter | the number of nodes | Pods that cannot be scheduled; unused nodes |
| KEDA | the number of Pods, down to zero | events, such as the length of a queue |

The HPA is part of Kubernetes and needs a source of metrics, usually metrics-server. VPA, node
autoscalers and KEDA are add-ons. Horizontal means more copies, vertical means bigger copies. A
DaemonSet cannot be scaled horizontally.

### Serverless and service mesh

- **Serverless** abstracts servers away from the user: the provider operates and scales the capacity,
  on demand and without action by the user, and charges by use. Functions as a Service is the
  best-known form; Knative brings the model to Kubernetes.
- A **service mesh** manages the traffic between services and adds reliability (retries, timeouts),
  observability and security (mutual TLS) uniformly, **without changes to application code**.
  Classically a proxy runs as a sidecar in every Pod, the data plane, configured by a control plane.
  Istio and Linkerd are the graduated examples.

### Open standards

Kubernetes stays replaceable piece by piece because its edges are interfaces, not products.

| Standard | Between | Examples of implementations |
|---|---|---|
| OCI image, runtime and distribution specs | image format, low-level runtime, registry | runc, any OCI registry |
| CRI, Container Runtime Interface | kubelet and container runtime (gRPC) | containerd, CRI-O |
| CNI, Container Network Interface | runtime and network plugin | Calico, Cilium, Flannel |
| CSI, Container Storage Interface | Kubernetes and storage systems | drivers of clouds and storage vendors |
| SMI, Service Mesh Interface | a common API for service meshes | the project is archived |

### Community and collaboration

The **CNCF** is a vendor-neutral foundation, part of the Linux Foundation, that hosts Kubernetes and
many other projects. It does not develop them; the projects govern themselves.

| Maturity level | Meaning |
|---|---|
| Sandbox | early and experimental; breaking changes are expected |
| Incubating | gaining adoption, becoming stable; used in production |
| Graduated | highly mature, widely adopted, with proven governance and security practices |
| Archived | inactive, no longer recommended |

- The **Technical Oversight Committee (TOC)** is the technical governing body: it admits projects and
  decides when one moves up a level. The **Governing Board** handles business, marketing and budget.
  **Technical Advisory Groups (TAGs)** contribute expertise in areas such as security or
  infrastructure.
- Inside the Kubernetes project, **SIGs** (Special Interest Groups, such as SIG Network or SIG
  Storage) own the parts of the project, working groups handle topics that cross SIGs, and the
  Steering Committee oversees the whole. A larger change starts as a **KEP**, a Kubernetes Enhancement
  Proposal, sponsored by a SIG.

| Persona | Concern |
|---|---|
| DevOps engineer | one team owns an application from development through operation |
| Site reliability engineer (SRE) | reliability of what runs in production, by means of software engineering: SLOs, automation |
| Platform engineer | builds the self-service platform that developers deploy on |
| Cloud architect, security engineer, developer | design of the whole, protection of it, the applications on it |

## Common confusions

- Monitoring tells you that something is wrong; observability lets you ask why.
- Prometheus is a backend that pulls metrics; OpenTelemetry produces and transports telemetry and
  stores nothing.
- The SLO is the internal target, the SLA the external promise, and the SLA is usually the weaker one.
- HPA and Cluster Autoscaler work together: one creates Pods, the other makes room for them.
- "Graduated" describes the maturity of a project, not the quality of a release; "sandbox" projects
  are CNCF projects too.
