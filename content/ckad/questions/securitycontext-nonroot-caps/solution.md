Look at the starting point first, inside the container:

```bash
ssh {{host}}
k -n app-puma exec deploy/edge -- sh -c 'id; grep -E "NoNewPrivs|Cap(Bnd|Eff)" /proc/1/status'
# uid=0(root) gid=0(root) ...
# NoNewPrivs:  0
# CapBnd:      00000000a80435fb       <- the runtime's default set plus NET_ADMIN
k -n app-puma edit deploy edge
```

```yaml
      containers:
        - name: httpd
          securityContext:
            runAsUser: 10001
            runAsGroup: 10001
            runAsNonRoot: true
            allowPrivilegeEscalation: false
            capabilities:
              drop: [ALL]
              add: [NET_BIND_SERVICE]       # replaces the old list with NET_ADMIN
```

`runAsUser`, `runAsGroup` and `runAsNonRoot` may also be set once in the Pod-level
`spec.securityContext`. `allowPrivilegeEscalation` and `capabilities` exist only per container.

```bash
k -n app-puma rollout status deploy edge
k -n app-puma exec deploy/edge -- sh -c 'id; grep -E "NoNewPrivs|Cap(Bnd|Eff)" /proc/1/status'
# uid=10001 gid=10001 ...
# NoNewPrivs:  1
# CapBnd:      0000000000000400       <- only bit 10 = NET_BIND_SERVICE
# CapEff:      0000000000000000
k -n app-puma exec deploy/edge -- wget -qO- 127.0.0.1:80        # puma edge ok
```

**Reading the result**

- `drop: [ALL]` plus `add` is the pattern to remember. Dropping single names leaves the rest of the
  default set (CHOWN, SETUID, NET_RAW, ...) in place. Names are written without `CAP_`.
- `runAsNonRoot: true` changes no UID. It makes the kubelet refuse a container that would run as 0,
  with `CreateContainerConfigError`. The UID comes from `runAsUser` or the image.
- `CapEff` is empty: Kubernetes puts added capabilities into the bounding set, and a process that
  is not root does not get them as effective capabilities. The server can still bind port 80
  because the container runtime of this cluster allows unprivileged processes to use low ports.
  Where it does not, a non-root server has to listen on a port above 1023.
