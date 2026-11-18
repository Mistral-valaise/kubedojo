kubectl delete namespace app-skunk --ignore-not-found --wait >/dev/null
kubectl create namespace app-skunk >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata: {name: api, namespace: app-skunk, labels: {app: orders}}
spec:
  replicas: 2
  selector: {matchLabels: {app: orders, tier: api}}
  template:
    metadata: {labels: {app: orders, tier: api}}
    spec:
      containers:
        - {name: nginx, image: "nginx:1.27-alpine", ports: [{containerPort: 80}]}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: worker, namespace: app-skunk, labels: {app: orders}}
spec:
  replicas: 2
  selector: {matchLabels: {app: orders, tier: worker}}
  template:
    metadata: {labels: {app: orders, tier: worker}}
    spec:
      terminationGracePeriodSeconds: 1
      containers:
        - {name: worker, image: "busybox:1.36", command: ["sh", "-c", "while true; do echo processing queue; sleep 30; done"]}
---
apiVersion: v1
kind: Service
metadata: {name: orders, namespace: app-skunk, labels: {app: orders}}
spec:
  selector: {app: orders}
  ports: [{name: http, port: 80, targetPort: 80}]
YAML
kubectl -n app-skunk rollout status deploy/api --timeout=180s >/dev/null 2>&1 || true
kubectl -n app-skunk rollout status deploy/worker --timeout=180s >/dev/null 2>&1 || true
