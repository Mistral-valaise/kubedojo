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

The reason is in the container's own log. `transform` is waiting to be restarted, so `logs` shows
the run that just crashed:

```bash
k -n team-magpie logs batch-worker -c transform
# loading schema
# FATAL: unsupported schema version 7
```

If you only see `loading schema`, the container has just been started again and has not failed yet.
Read the run before it with `--previous`, or wait a few seconds:

```bash
k -n team-magpie logs batch-worker -c transform --previous
```

`--previous` always means "the run before the one `logs` shows". While the container is waiting,
that older run has usually been removed already and kubectl answers
`unable to retrieve container logs` — then leave `--previous` away.

Write the answers:

```bash
echo transform > /opt/course/magpie/container
echo 42 > /opt/course/magpie/exit-code
k -n team-magpie logs batch-worker -c transform | tail -1 > /opt/course/magpie/reason
cat /opt/course/magpie/reason     # must be the FATAL line
```

Or, for the exit code, without reading:

```bash
k -n team-magpie get pod batch-worker \
  -o jsonpath='{.status.containerStatuses[?(@.name=="transform")].lastState.terminated.exitCode}'
```
