# kubedojo

**Pass your Kubernetes exam by doing, not by reading.**

kubedojo is a hands-on training platform for the **CKA** (Certified Kubernetes Administrator) and
**CKS** (Certified Kubernetes Security Specialist) exams. You get real kubeadm clusters, a real terminal
in your browser and tasks in exam format — graded automatically, with an explanation for every point
you miss.

- **Learn** — one short lesson per exam domain: the 20% of knowledge that solves 80% of the tasks.
- **Practise** — solve single questions on a live cluster. Check your work at any time, get tiered hints,
  read the worked solution, reset and try again.
- **Simulate** — a timed exam (120 minutes, weighted like the real one, no hints). At the end you get a
  score per domain, pass or fail, and what exactly was wrong in each task.
- **Track** — your readiness per domain and a recommendation what to practise next.

58 original questions ship today (37 CKA, 21 CKS). Every one is proven solvable by an automated test
against a real cluster.

## Quick start (local)

You need [Docker](https://docs.docker.com/get-docker/) (Docker Desktop, OrbStack or Colima) with at least
4 GB of memory — 8 GB for full exams — and [kind](https://kind.sigs.k8s.io/).

```bash
brew install kind            # macOS; see the kind docs for Linux and Windows
git clone https://github.com/kubedojo/kubedojo && cd kubedojo
make build                   # needs Go 1.26+ and Node 22+
./bin/kubedojo up
```

`kubedojo up` checks your machine, builds the two sandbox images on first run (a few minutes, once) and
opens <http://127.0.0.1:4400>. Pick a track, open a question, and a cluster is created for you in about
half a minute.

```text
kubedojo up        Start kubedojo locally and open it in your browser
kubedojo doctor    Check that this machine can run sandboxes
kubedojo down      Remove every sandbox kubedojo created
kubedojo content lint | test    Validate content / prove every question against a real cluster
```

Useful flags for `up`: `--observability` (local Grafana with dashboards), `--addr`, `--no-open`,
`--kubernetes-version`. Set `ANTHROPIC_API_KEY` to enable the optional AI tutor.

macOS is the primary platform. Linux works the same way. On Windows, run kubedojo inside WSL 2 with
Docker Desktop's WSL integration; a native Windows build compiles but is not tested yet.

## How it works

```text
Browser (React, xterm.js)
   │ REST + WebSocket
kubedojo server (Go, single binary, UI embedded) ── SQLite (history, progress)
   │ Docker API
Sandbox per session:  candidate@terminal  ──ssh──▶  cka1234 (control plane + workers)
                                                    cka5678 ...
```

Each cluster is a set of `kindest/node` containers: real systemd, kubelet, kubeadm, etcd and static Pods,
extended with sshd and a `candidate` user. That is enough to break and repair a kubelet, edit the API
server manifest, back up etcd or encrypt Secrets at rest — like on the exam.

A question is a directory with a task, checks, a setup script and a reference solution
([docs/writing-questions.md](docs/writing-questions.md)). Checks are plain commands with an expected
result, so the grader never knows more than you could find out yourself — and the UI shows the command
behind every check.

## Run it for your team (Kubernetes)

```bash
helm install kubedojo deploy/helm/kubedojo -n kubedojo --create-namespace \
  --set ingress.enabled=true --set ingress.host=kubedojo.example.com \
  --set oidc.enabled=true --set oidc.issuerURL=... --set oidc.clientID=... --set oidc.existingSecret=kubedojo-oidc \
  --set sandbox.nodeSelector."kubedojo\.dev/pool"=sandbox --set sandbox.tolerationKey=kubedojo.dev/sandbox \
  --set telemetry.otlpEndpoint=http://alloy.monitoring.svc:4318
```

In hosted mode every session gets its own Pod running Docker-in-Docker, in which the same sandbox is
created. **Those Pods are privileged.** Read [docs/deployment.md](docs/deployment.md) before you expose
kubedojo: it covers the dedicated node pool, the NetworkPolicy, authentication and sizing.

## Observability

kubedojo speaks OpenTelemetry. Point `OTEL_EXPORTER_OTLP_ENDPOINT` (or `telemetry.otlpEndpoint` in the
chart) at an OTLP/HTTP receiver such as Grafana Alloy and you get traces (Tempo), metrics (Mimir or
Prometheus) and logs (Loki). The chart ships three Grafana dashboards — platform health, sessions &
sandboxes, learning analytics — and alert rules. Try them locally:

```bash
./bin/kubedojo up --observability     # Grafana at http://localhost:3000
```

Details and the metric reference: [docs/observability.md](docs/observability.md).

## Scope and limits

- The kind driver covers everything that works on a container-based node. Tasks that need a real kernel
  boundary or a package manager on the node — AppArmor, gVisor, Falco with eBPF, `apt`-based kubeadm
  upgrades — are not included yet. They are planned for a VM driver; the driver interface is in place.
- Hosted mode is single-tenant: one team that trusts each other. It is not a hardened multi-tenant
  service.
- Kubernetes moves fast and so do the exams. Lessons and questions target Kubernetes v1.36.

## Contributing

Questions and lessons are plain files under [`content/`](content). New questions are very welcome —
`kubedojo content test --question <id>` tells you whether yours works.

All content is original. Please do not submit questions copied from commercial exam simulators or from
the real exams.

kubedojo is not affiliated with or endorsed by the Linux Foundation or the CNCF. CKA and CKS are
trademarks of the Linux Foundation.

## License

[Apache-2.0](LICENSE)
