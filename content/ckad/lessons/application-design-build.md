# Application Design and Build

**20% of the exam.** Four topics, all of them typing work: images, Jobs and CronJobs, multi-container
Pods, volumes. Speed comes from generating the skeleton and knowing where each field goes.

## The 20% that solves 80%

### Images

In a Dockerfile, know `FROM`, `COPY`, `RUN`, `ENV`, `USER` (the last one decides who runs the
process) and `CMD`.

```bash
podman build -t shop:1.2 .                       # -t name:tag, the dot is the build context
podman image ls; podman image inspect shop:1.2
podman tag shop:1.2 registry.example/team/shop:1.2 && podman push registry.example/team/shop:1.2
podman save -o /path/shop.tar shop:1.2           # archive; --format oci-archive if asked
```

An edited Dockerfile changes nothing until you rebuild, and an archive is a snapshot: save again after
a rebuild. `docker` takes the same arguments.

### Jobs

```bash
k create job report --image=busybox:1.36 $do -- sh -c 'echo done' > job.yaml
```

```yaml
spec:
  completions: 6                 # Pods that must succeed
  parallelism: 3                 # at the same time
  backoffLimit: 2                # failures before the Job is Failed (default 6)
  activeDeadlineSeconds: 30      # kill the whole Job after 30s
  ttlSecondsAfterFinished: 60    # delete it 60s after it finished
  template:
    spec:
      restartPolicy: Never       # Never or OnFailure; Always is rejected
```

`k wait --for=condition=complete job/report`, `k logs job/report`, `k get pods -l job-name=report`.
The Pod template of a Job is immutable: fix a mistake with `k delete job` and a new apply.

### CronJobs

```bash
k create cronjob sync --image=busybox:1.36 --schedule="*/15 * * * *" $do -- sh -c 'date' > cj.yaml
k create job sync-test --from=cronjob/sync       # run it now instead of waiting
```

Three nested specs. Most lost points are a field at the wrong level:

| Level | Fields |
|---|---|
| `spec` (CronJob) | `schedule`, `concurrencyPolicy` (`Allow`/`Forbid`/`Replace`), `startingDeadlineSeconds`, `successfulJobsHistoryLimit` (3), `failedJobsHistoryLimit` (1), `suspend` |
| `spec.jobTemplate.spec` (Job) | `completions`, `parallelism`, `backoffLimit`, `activeDeadlineSeconds` |
| `...template.spec` (Pod) | `containers`, `restartPolicy`, `volumes` |

Schedule: `minute hour day-of-month month day-of-week`; `30 2 * * 1-5` is 02:30 on weekdays.

### Multi-container Pods

| | Starts | Must exit? | Use |
|---|---|---|---|
| init container | before the app, one after another | yes | prepare files, wait for a dependency |
| sidecar (init container with `restartPolicy: Always`) | before the app | no | log shipper, proxy |
| second regular container | together with the app | no | helper without start order |

Containers of a Pod share the network (`localhost`) but **not** the filesystem. Files travel through
a volume mounted into both, at whatever path each one needs:

```yaml
spec:
  volumes:
    - {name: work, emptyDir: {}}
  initContainers:
    - name: prepare
      image: busybox:1.36
      command: ["sh", "-c", "echo hello > /work/index.html"]
      volumeMounts: [{name: work, mountPath: /work}]
  containers:
    - name: web
      image: nginx:1.27-alpine
      volumeMounts: [{name: work, mountPath: /usr/share/nginx/html}]
```

`Init:0/1`, `Init:Error`: read `k logs <pod> -c <init-container>`.

### Volumes from the developer's side

- `emptyDir: {}` — scratch space for the lifetime of the Pod; `{medium: Memory, sizeLimit: 64Mi}` for tmpfs.
- `persistentVolumeClaim: {claimName: data}` — data that outlives the Pod. There is no generator for
  the claim; its spec is `accessModes: [ReadWriteOnce]` and `resources: {requests: {storage: 200Mi}}`,
  and without `storageClassName` it uses the default class.
- A claim of a class with `WaitForFirstConsumer` stays `Pending` until a Pod uses it. That is normal.

## Habits that save points

- `$do` (`--dry-run=client -o yaml`) for every Job, CronJob, Pod and Deployment; write by hand only what the generator cannot.
- `k explain cronjob.spec.jobTemplate.spec` beats guessing the level of a field.
- `volumes` (where it comes from) and `volumeMounts` (where the container sees it) are linked by `name`; a typo leaves the Pod uncreated with a clear error.
- Prove it before moving on: `k logs job/...`, `k exec ... -- mount`, `curl <ClusterIP>`.
