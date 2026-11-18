kubectl delete namespace app-weasel --ignore-not-found --wait >/dev/null
kubectl create namespace app-weasel >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: v1
kind: ConfigMap
metadata: {name: telemetry-nginx, namespace: app-weasel}
data:
  default.conf: |
    server {
      listen 8080;
      location / { default_type text/plain; return 200 "telemetry web ok\n"; }
    }
    server {
      listen 9113;
      location / { default_type text/plain; return 200 "telemetry_up 1\n"; }
    }
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: telemetry, namespace: app-weasel}
spec:
  replicas: 2
  selector: {matchLabels: {app: telemetry}}
  template:
    metadata: {labels: {app: telemetry}}
    spec:
      containers:
        - name: telemetry
          image: nginx:1.27-alpine
          ports:
            - {name: http, containerPort: 8080}
            - {name: metrics, containerPort: 9113}
          volumeMounts:
            - {name: conf, mountPath: /etc/nginx/conf.d}
      volumes:
        - name: conf
          configMap: {name: telemetry-nginx}
YAML
kubectl -n app-weasel rollout status deploy/telemetry --timeout=180s >/dev/null 2>&1 || true
