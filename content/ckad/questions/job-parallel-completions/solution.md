```bash
ssh {{host}}
k -n app-bobcat create job thumbnailer --image=busybox:1.36 $do \
  -- sh -c 'sleep 2; echo "rendered by $HOSTNAME"' > job.yaml
vim job.yaml
```

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: thumbnailer
  namespace: app-bobcat
spec:
  completions: 6            # <- add
  parallelism: 3            # <- add
  backoffLimit: 2           # <- add
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: render      # <- the generator names it thumbnailer
          image: busybox:1.36
          command: ["sh", "-c", "sleep 2; echo \"rendered by $HOSTNAME\""]
```

Single quotes around the script on the command line keep **your** shell from expanding `$HOSTNAME`; it
must reach the container unexpanded.

```bash
k apply -f job.yaml
k -n app-bobcat get pods -w                                   # three at a time
k -n app-bobcat wait --for=condition=complete job/thumbnailer --timeout=120s
k -n app-bobcat get job thumbnailer                           # COMPLETIONS 6/6
k -n app-bobcat logs -l job-name=thumbnailer                  # rendered by thumbnailer-xxxxx

k -n app-bobcat get pods -l job-name=thumbnailer \
  -o custom-columns=:metadata.name --no-headers > /opt/course/bobcat/pods
```

**Takeaways:** `completions` = how many successes, `parallelism` = how many at once, `backoffLimit` =
how many failures before the Job gives up. The Pod template of a Job cannot be edited afterwards: to fix
a mistake, `k delete job thumbnailer` and apply again. Pods of a Job are found by the label
`job-name=<job>`.
