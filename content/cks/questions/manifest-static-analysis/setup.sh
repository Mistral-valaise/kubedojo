mkdir -p /opt/course/static
cat > /opt/course/static/deployment.yaml <<'YAML'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: exporter
  namespace: sec-sandpiper
  labels: {app: exporter}
spec:
  replicas: 2
  selector:
    matchLabels: {app: exporter}
  template:
    metadata:
      labels: {app: exporter}
    spec:
      hostNetwork: true
      containers:
        - name: exporter
          image: nginx:latest
          ports:
            - containerPort: 8080
          securityContext:
            privileged: true
          volumeMounts:
            - name: runtime
              mountPath: /run/containerd/containerd.sock
            - name: cache
              mountPath: /var/cache/nginx
      volumes:
        - name: runtime
          hostPath:
            path: /run/containerd/containerd.sock
        - name: cache
          emptyDir: {}
YAML
chown -R candidate:candidate /opt/course/static
