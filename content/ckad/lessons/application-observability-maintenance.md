# Application Observability and Maintenance

**15% of the exam.** Probes, reading what a broken workload tells you, and outdated API versions.

## The 20% that solves 80%

### Three probes, three questions

| Probe | Question | On failure |
|---|---|---|
| `startupProbe` | has it started yet? | container is restarted after the budget; until it succeeds the other two do not run |
| `livenessProbe` | is it still alive? | container is restarted |
| `readinessProbe` | may it get traffic? | Pod is removed from the Service endpoints, **no** restart |

```yaml
containers:
  - name: app
    image: nginx:1.27-alpine
    ports: [{name: http, containerPort: 80}]
    startupProbe:                       # budget = failureThreshold × periodSeconds = 60s
      httpGet: {path: /healthz, port: http}
      periodSeconds: 5
      failureThreshold: 12
    livenessProbe:
      tcpSocket: {port: 80}
      initialDelaySeconds: 10
      periodSeconds: 10
    readinessProbe:
      exec: {command: ["cat", "/tmp/ready"]}
      periodSeconds: 5
```

- One handler per probe: `httpGet` (2xx/3xx is success), `tcpSocket`, `exec` (exit code 0), `grpc`.
- Timings are **siblings** of the handler: `initialDelaySeconds` (0), `periodSeconds` (10),
  `timeoutSeconds` (1), `failureThreshold` (3).
- No imperative command exists: `k edit`, and `k explain pod.spec.containers.livenessProbe` for the names.
- Slow start → startup probe, not a huge `initialDelaySeconds` on the liveness probe.
- Verify with `k describe pod`: the `Liveness:` line and the Events `Unhealthy` / `Killing`.

### The Pod status picks the tool

| Status | Meaning | Look at |
|---|---|---|
| `Pending` | not scheduled, or a volume is missing | `k describe pod` → Events |
| `ImagePullBackOff`, `ErrImagePull` | image name, tag or registry | Events |
| `CreateContainerConfigError` | ConfigMap/Secret or a key of it is missing | Events |
| `CrashLoopBackOff`, `Error` | the process started and exited | `k logs`, exit code in `describe` |
| `Running` but `0/1` | readiness probe fails | Events, then the app |
| `Running` `1/1`, no answer | Service selector, `targetPort`, NetworkPolicy | `k get endpointslices` |

```bash
k describe pod web-7d4b9                       # State / Last State / Exit Code / Events
k get events --sort-by=.lastTimestamp          # everything in the Namespace, newest last
k events --for pod/web-7d4b9                   # only this object
```

### Logs and exec

```bash
k logs web-7d4b9                    # current run (the last crashed run while in CrashLoopBackOff)
k logs web-7d4b9 --previous         # the run before the current one
k logs web-7d4b9 -c sidecar         # one container; --all-containers for all of them
k logs deploy/web --tail=20 -f      # pick a Pod of the Deployment, follow; -l app=web for all Pods
k exec web-7d4b9 -- env             # one command; -it ... -- sh for a shell
k exec deploy/web -- wget -qO- -T 3 http://backend:8080    # test a Service from inside
```

### Ephemeral containers and resource usage

Minimal images have no shell. `k debug` adds a container to a **running** Pod; it cannot be removed
again and has no probes or ports.

```bash
k debug -it web-7d4b9 --image=busybox:1.36 --target=web      # shares the process namespace of "web"
k debug web-7d4b9 -it --image=busybox:1.36 --copy-to=web-dbg --share-processes   # debug a copy
k top pod --sort-by=memory -A ; k top pod web-7d4b9 --containers ; k top node
```

`k top` needs metrics-server. kubedojo clusters run **without** it (`Metrics API not available`, an
HPA shows `<unknown>`); in the exam it is installed, so know `--sort-by`, `--containers` and `-l`.

### API deprecations

Beta versions get removed; old manifests then fail with `no matches for kind "CronJob" in version "batch/v1beta1"`.

```bash
k api-resources | grep -i cronjob             # which group/version serves the kind now
k explain hpa.spec.metrics.resource.target    # the current schema of a field
k apply --dry-run=server -f old.yaml          # ask the server, change nothing
```

`apiVersion` is `group/version` (`apps/v1`, `batch/v1`), for the core group only `v1`. A new version
may differ in shape: `apps/v1` requires `spec.selector`, `autoscaling/v2` nests the CPU target under
`target:`. Fix the version, dry-run, read the next message.

## Habits that save points

- Stacked faults are normal: after every fix run `k get pod` again.
- Probes and env changes roll out new Pods — finish with `k rollout status deploy/<name>`.
- Copy log evidence **before** fixing; the crashing Pod and its log disappear with the rollout.
- Named ports (`targetPort: http`) resolve in the Pod: check `containerPort` there, not only the Service.
