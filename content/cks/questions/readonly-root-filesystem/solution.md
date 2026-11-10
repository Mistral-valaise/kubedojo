```bash
ssh {{host}}
k -n sec-osprey edit deploy cache
```

```yaml
    spec:
      volumes:
        - name: cache
          emptyDir: {}
        - name: run
          emptyDir: {}
      containers:
        - name: nginx
          image: nginx:1.27-alpine
          securityContext:
            readOnlyRootFilesystem: true
            allowPrivilegeEscalation: false
          volumeMounts:
            - name: cache
              mountPath: /var/cache/nginx
            - name: run
              mountPath: /var/run
```

```bash
k -n sec-osprey rollout status deploy cache
k -n sec-osprey exec deploy/cache -- touch /etc/x          # Read-only file system
k -n sec-osprey exec deploy/cache -- wget -qO- 127.0.0.1 | head -4
```

**If you do not know the writable paths:** set only the read-only flag and read the crash log —

```text
nginx: [emerg] mkdir() "/var/cache/nginx/client_temp" failed (30: Read-only file system)
nginx: [emerg] open() "/var/run/nginx.pid" failed (30: Read-only file system)
```

— add one `emptyDir` per reported path until the container starts.
