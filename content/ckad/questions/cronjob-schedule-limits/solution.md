Generate the skeleton, then add what the imperative command cannot set:

```bash
ssh {{host}}
k -n app-beaver create cronjob report-sync --image=busybox:1.36 --schedule="*/15 * * * *" \
  $do -- sh -c "echo report synced" > cj.yaml
vim cj.yaml
```

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: report-sync
  namespace: app-beaver
spec:
  schedule: "*/15 * * * *"
  concurrencyPolicy: Forbid                # skip a run while the last one is active
  startingDeadlineSeconds: 45              # too late by more than 45s: do not start
  successfulJobsHistoryLimit: 2
  failedJobsHistoryLimit: 4
  jobTemplate:
    spec:
      activeDeadlineSeconds: 30            # a property of the Job, not of the CronJob
      template:
        spec:
          restartPolicy: OnFailure
          containers:
            - name: sync                   # the generator names it report-sync
              image: busybox:1.36
              command: ["sh", "-c", "echo report synced"]
```

```bash
k apply -f cj.yaml
k -n app-beaver create job report-sync-manual --from=cronjob/report-sync
k -n app-beaver get job report-sync-manual          # COMPLETIONS 1/1
k -n app-beaver logs job/report-sync-manual         # report synced
```

**Three levels, three specs:** `spec` (CronJob: schedule, concurrency, history, starting deadline) →
`jobTemplate.spec` (Job: completions, parallelism, backoffLimit, activeDeadlineSeconds) →
`template.spec` (Pod: containers, restartPolicy). Most lost points in CronJob tasks are a field at the
wrong level; `k explain cronjob.spec.jobTemplate.spec` settles it in seconds.
