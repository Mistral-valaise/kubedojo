Three mistakes are stacked, and each one hides the next. Fix what the cluster reports, look again.

```bash
ssh {{host}}
k -n app-moose get pod
# orders-...   0/1   CreateContainerConfigError
k -n app-moose describe pod -l app=orders | tail -4
# Error: couldn't find key log_level in ConfigMap app-moose/orders-config
k -n app-moose describe cm orders-config          # keys: LOG_LEVEL, app.conf
```

**Mistake 1 — wrong key.** Keys are case-sensitive.

```bash
k -n app-moose edit deploy orders                 # configMapKeyRef.key: LOG_LEVEL
k -n app-moose get pod
# orders-...   0/1   CrashLoopBackOff   2 (11s ago)
```

**Mistake 2 — the process exits.** Now there is a container, so there are logs:

```bash
k -n app-moose logs deploy/orders
# orders-api starting, log level info
# FATAL: config file /etc/orders/orders.conf not found
```

If kubectl only prints the first line or an error, the container is between two runs: repeat the
command, or add `--previous` while the container is running again. Save the evidence **now** —
once the fix is rolled out, the crashing Pods are gone:

```bash
k -n app-moose logs deploy/orders | grep FATAL > /opt/course/moose/root-cause
cat /opt/course/moose/root-cause                  # must not be empty
```

The Deployment mounts ConfigMap `orders-config` at `/etc/orders` with the single item `app.conf`,
but `CONFIG_FILE` points to `orders.conf`:

```bash
k -n app-moose edit deploy orders                 # CONFIG_FILE: /etc/orders/app.conf
k -n app-moose rollout status deploy orders
k -n app-moose get pod                            # 1/1 Running
```

**Mistake 3 — running, but not reachable.**

```bash
k -n app-moose exec deploy/orders -- wget -qO- -T 3 http://orders
# wget: can't connect to remote host: Connection refused
k -n app-moose get endpointslices                 # endpoints exist, PORTS 80
k -n app-moose logs deploy/orders | tail -1       # orders-api listening on :8080
k -n app-moose exec deploy/orders -- wget -qO- -T 3 http://localhost:8080   # orders-api ok
```

The Service uses `targetPort: http`. That name is looked up in the Pod, where the developer
declared `name: http, containerPort: 80`. The Service is correct, the Deployment is not:

```bash
k -n app-moose edit deploy orders                 # containerPort: 8080
k -n app-moose rollout status deploy orders
k -n app-moose exec deploy/orders -- wget -qO- -T 3 http://orders          # orders-api ok
```

**Takeaway:** the Pod status picks the tool. `CreateContainerConfigError`/`ImagePullBackOff` →
`describe pod` (Events). `CrashLoopBackOff`/`Error` → `logs` (and `--previous`). `Running` but
unreachable → endpoints and ports, tested from inside with `exec`.
