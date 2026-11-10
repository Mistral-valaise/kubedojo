```bash
ssh {{host}}
k label ns sec-magpie pod-security.kubernetes.io/enforce=restricted
# Warning: existing pods in namespace "sec-magpie" violate the new PodSecurity enforce level "restricted:latest"
# Warning: web-...: allowPrivilegeEscalation != false, unrestricted capabilities, runAsNonRoot != true, seccompProfile
```

The warning already lists what is missing. The same list appears when the ReplicaSet tries to create a Pod:

```bash
k -n sec-magpie rollout restart deploy web
k -n sec-magpie describe rs | grep -A3 FailedCreate
```

Fix the Pod template (`k -n sec-magpie edit deploy web`):

```yaml
      containers:
        - name: web
          image: nginxinc/nginx-unprivileged:1.27-alpine
          securityContext:
            allowPrivilegeEscalation: false
            runAsNonRoot: true
            capabilities:
              drop: [ALL]
            seccompProfile:
              type: RuntimeDefault
```

```bash
k -n sec-magpie rollout status deploy web
k -n sec-magpie get pod                     # 2 new Pods Running
```

`runAsNonRoot: true` only *verifies*: the image must define a non-root user (this one runs as UID 101),
otherwise add `runAsUser: <uid>`.

Useful while migrating: set `warn` and `audit` to the target level first and `enforce` last —
`k label ns <ns> pod-security.kubernetes.io/warn=restricted`.
