set -e
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
      containers:
        - name: exporter
          image: nginx:1.27-alpine
          ports:
            - containerPort: 8080
          securityContext:
            privileged: false
            allowPrivilegeEscalation: false
          volumeMounts:
            - name: cache
              mountPath: /var/cache/nginx
      volumes:
        - name: cache
          emptyDir: {}
YAML
kubectl apply --dry-run=client -f /opt/course/static/deployment.yaml >/dev/null
