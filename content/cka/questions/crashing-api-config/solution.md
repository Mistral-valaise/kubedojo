Two faults are stacked. Fix what the cluster reports, then look again.

```bash
ssh {{host}}
k -n team-raven get pod
# api-...   0/1   CreateContainerConfigError
k -n team-raven describe pod -l app=api | tail -5
# Error: couldn't find key api-token in Secret team-raven/api-credentials
k -n team-raven describe secret api-credentials
# Data: token: 13 bytes
```

**Fault 1:** the key is called `token`.

```bash
k -n team-raven edit deploy api      # secretKeyRef.key: token
k -n team-raven get pod -w
# api-...   1/1   Running   2 (3s ago)      <- restarts keep counting
k -n team-raven describe pod -l app=api | grep -i liveness
# Liveness probe failed: Get "http://10.244.1.7:8080/": connect: connection refused
```

**Fault 2:** nginx listens on port 80, the probe asks port 8080.

```bash
k -n team-raven edit deploy api      # livenessProbe.httpGet.port: 80
k -n team-raven rollout status deploy api
```

**Takeaway:** the Pod status names the phase that fails.
`CreateContainerConfigError` → configuration objects (ConfigMap/Secret/keys).
`Running` with climbing restarts → probes or the process itself (`logs --previous`).
