# Workloads & Scheduling

**15% of the exam.** Deployments and their rollouts, configuration, autoscaling, and how Pods land on nodes.

## The 20% that solves 80%

### Generate, do not type

```bash
export do="--dry-run=client -o yaml"
k create deploy web --image=nginx:1.27-alpine --replicas=3 $do > web.yaml
k run tmp --image=busybox:1.36 $do --command -- sleep 1d > pod.yaml
k create cm app --from-literal=MODE=prod --from-file=app.conf
k create secret generic db --from-literal=password=s3cret
k create job once --image=busybox:1.36 -- date
k create cronjob tick --image=busybox:1.36 --schedule="*/5 * * * *" -- date
```

There is no generator for DaemonSets: generate a Deployment, change `kind`, remove `replicas` and `strategy`.

### Rollouts

```bash
k set image deploy/web nginx=nginx:1.28-alpine
k rollout status deploy/web
k rollout history deploy/web
k rollout undo deploy/web [--to-revision=2]
k scale deploy/web --replicas=5
```

`maxSurge` / `maxUnavailable` control how fast and how safely a RollingUpdate proceeds.

### Configuration into Pods

```yaml
env:
  - name: MODE
    valueFrom: {configMapKeyRef: {name: app, key: MODE}}
envFrom:
  - secretRef: {name: db}
volumeMounts:
  - {name: conf, mountPath: /etc/app, readOnly: true}
volumes:
  - name: conf
    configMap: {name: app}
```

### Requests, limits and QoS

The scheduler uses **requests**; the kubelet enforces **limits**. QoS decides who is evicted first under
node pressure:

| QoS class | Condition | Evicted |
|---|---|---|
| `BestEffort` | no requests or limits at all | first |
| `Burstable` | anything in between | second |
| `Guaranteed` | requests == limits for every container | last |

### Steering the scheduler

| Tool | Says |
|---|---|
| `nodeName` | run exactly here (bypasses the scheduler) |
| `nodeSelector` / node affinity | I want nodes with these labels |
| taints + tolerations | this node repels Pods unless they tolerate it |
| pod (anti-)affinity | place me near / away from these Pods (`topologyKey`) |
| `topologySpreadConstraints` | spread me evenly |

A toleration **allows** a Pod onto a tainted node; it does not attract it. To force a Pod onto control
plane nodes you need both: toleration for `node-role.kubernetes.io/control-plane:NoSchedule` **and** a
nodeSelector.

"One Pod per node, but as a Deployment" = required pod anti-affinity with
`topologyKey: kubernetes.io/hostname` (or a topology spread constraint with `maxSkew: 1` and
`whenUnsatisfiable: DoNotSchedule`).

### Autoscaling

```bash
k autoscale deploy web --min=2 --max=6 --cpu-percent=70
k top pod --containers ; k top node           # needs metrics-server
```

HPA needs CPU **requests** on the containers — utilisation is a percentage of the request.

## Habits that save points

- `k explain deploy.spec.strategy --recursive | less` when you forget a field name.
- Multi-container Pods share network and volumes. Sidecars that should start first and keep running are
  `initContainers` with `restartPolicy: Always`.
- `k get pod -o wide` shows the node; `k get pod --show-labels` shows what selectors can match.
