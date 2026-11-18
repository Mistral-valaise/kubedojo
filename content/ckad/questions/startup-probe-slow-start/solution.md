```bash
ssh {{host}}
k -n app-lemur get pod
# ledger-...   0/1   Running            3 (20s ago)
# ledger-...   0/1   CrashLoopBackOff   3 (5s ago)
k -n app-lemur describe pod -l app=ledger | grep -A12 Events
# Warning  Unhealthy  Liveness probe failed: Get "http://10.244.1.9:8080/healthz": connect: connection refused
# Normal   Killing    Container ledger failed liveness probe, will be restarted
k -n app-lemur logs deploy/ledger
# ledger: replaying journal, this takes a while        <- never gets further
```

The liveness probe gives up after `periodSeconds 5 × failureThreshold 3 = 15s`, the app needs up
to 30s. The app is not broken, it is only slow to **start** — the case a startup probe exists for.
While the startup probe has not succeeded, liveness and readiness probes are switched off.

```bash
k -n app-lemur edit deploy ledger
```

```yaml
          startupProbe:
            httpGet:
              path: /healthz
              port: http              # or 8080
            periodSeconds: 5
            failureThreshold: 12      # 12 × 5s = 60s to start
          livenessProbe:              # unchanged
            httpGet: {path: /healthz, port: http}
            periodSeconds: 5
            failureThreshold: 3
```

```bash
k -n app-lemur rollout status deploy ledger
k -n app-lemur get pod             # 1/1 Running, RESTARTS 0
```

The new Pods are Ready after about 25 seconds, not after 60: the budget is an upper limit, the
first successful probe ends the startup phase.

**Takeaway:** three probes, three questions. Startup: *has it started yet?* (patient, runs once).
Liveness: *is it still alive?* (fast, restarts). Readiness: *may it get traffic?* (no restart).
Do not "fix" a slow start with a huge `initialDelaySeconds` or `failureThreshold` on the liveness
probe — that also delays the detection of a real hang later.
