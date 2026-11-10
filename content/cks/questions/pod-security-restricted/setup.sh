kubectl delete namespace sec-magpie --ignore-not-found --wait >/dev/null
kubectl create namespace sec-magpie >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata: {name: web, namespace: sec-magpie}
spec:
  replicas: 2
  selector: {matchLabels: {app: web}}
  template:
    metadata: {labels: {app: web}}
    spec:
      containers:
        - name: web
          image: nginxinc/nginx-unprivileged:1.27-alpine
          ports: [{containerPort: 8080}]
YAML
kubectl -n sec-magpie rollout status deployment web --timeout=180s >/dev/null 2>&1 || true
