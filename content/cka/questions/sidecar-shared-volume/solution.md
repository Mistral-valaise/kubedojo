```yaml
apiVersion: v1
kind: Pod
metadata:
  name: web-logger
  namespace: team-tern
spec:
  volumes:
    - name: logs
      emptyDir: {}
  initContainers:
    - name: log-tail
      image: busybox:1.36
      restartPolicy: Always                 # <- makes it a sidecar
      command: ["tail", "-F", "/var/log/nginx/access.log"]
      volumeMounts:
        - name: logs
          mountPath: /var/log/nginx
  containers:
    - name: web
      image: nginx:1.27-alpine
      volumeMounts:
        - name: logs
          mountPath: /var/log/nginx
```

```bash
ssh {{host}}
k apply -f pod.yaml
k -n team-tern get pod web-logger                       # READY 2/2
k -n team-tern exec web-logger -c web -- wget -qO- localhost >/dev/null
k -n team-tern logs web-logger -c log-tail
# 127.0.0.1 - - [10/Nov/2026:21:40:12 +0000] "GET / HTTP/1.1" 200 615 "-" "Wget" "-"
```

**Init container vs. sidecar vs. regular container:**

| | Starts | Must exit? | Restarted |
|---|---|---|---|
| init container | before the app, one after another | yes, before the next starts | per Pod `restartPolicy` |
| sidecar (init + `restartPolicy: Always`) | before the app | no, runs alongside | always |
| regular container | together with the others | no | per Pod `restartPolicy` |

`tail -F` (capital F) keeps retrying until the file exists, so it does not matter that nginx creates the
log file after the sidecar has started.
