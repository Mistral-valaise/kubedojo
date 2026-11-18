kubectl delete namespace app-cougar --ignore-not-found --wait >/dev/null
kubectl create namespace app-cougar >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata: {name: docs, namespace: app-cougar, labels: {app: docs}}
spec:
  replicas: 2
  selector: {matchLabels: {app: docs}}
  template:
    metadata: {labels: {app: docs}}
    spec:
      terminationGracePeriodSeconds: 2
      containers:
        - name: web
          image: nginx:1.27-alpine
          ports: [{containerPort: 80}]
          readinessProbe:
            httpGet: {path: /, port: 80}
            periodSeconds: 2
---
apiVersion: v1
kind: Service
metadata: {name: docs, namespace: app-cougar}
spec:
  selector: {app: docs}
  ports: [{port: 80, targetPort: 80}]
YAML
kubectl -n app-cougar rollout status deploy/docs --timeout=180s >/dev/null 2>&1 || true
