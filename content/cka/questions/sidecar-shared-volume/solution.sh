set -e
cat <<'YAML' | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata: {name: web-logger, namespace: team-tern}
spec:
  terminationGracePeriodSeconds: 1
  volumes:
    - {name: logs, emptyDir: {}}
  initContainers:
    - name: log-tail
      image: busybox:1.36
      restartPolicy: Always
      command: ["tail", "-F", "/var/log/nginx/access.log"]
      volumeMounts: [{name: logs, mountPath: /var/log/nginx}]
  containers:
    - name: web
      image: nginx:1.27-alpine
      volumeMounts: [{name: logs, mountPath: /var/log/nginx}]
YAML
