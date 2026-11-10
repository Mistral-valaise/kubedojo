```bash
ssh {{host}}
k -n team-magpie get pod batch-worker
# batch-worker   2/3   CrashLoopBackOff   4 (20s ago)
k -n team-magpie describe pod batch-worker
```

In the `Containers:` section each container has its own state. Two are `Running`; `transform` shows:

```text
  transform:
    State:          Waiting
      Reason:       CrashLoopBackOff
    Last State:     Terminated
      Reason:       Error
      Exit Code:    42
```

The reason is in the log of the **previous** run:

```bash
k -n team-magpie logs batch-worker -c transform --previous
# loading schema
# FATAL: unsupported schema version 7
```

If `--previous` reports that the old container is gone, the current run prints the same line a few
seconds after it starts — `k -n team-magpie logs batch-worker -c transform` works as well.

Write the answers:

```bash
echo transform > /opt/course/magpie/container
echo 42 > /opt/course/magpie/exit-code
k -n team-magpie logs batch-worker -c transform --previous | tail -1 > /opt/course/magpie/reason
```

Or, for the exit code, without reading:

```bash
k -n team-magpie get pod batch-worker \
  -o jsonpath='{.status.containerStatuses[?(@.name=="transform")].lastState.terminated.exitCode}'
```
