set -e
cat <<'YAML' | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-green
  namespace: app-gecko
spec:
  replicas: 3
  selector:
    matchLabels: {app: api, color: green}
  template:
    metadata:
      labels: {app: api, color: green}
    spec:
      containers:
        - name: api
          image: nginx:1.27-alpine
          ports:
            - containerPort: 80
          readinessProbe:
            httpGet: {path: /, port: 80}
            periodSeconds: 2
YAML
kubectl -n app-gecko rollout status deployment/api-green --timeout=120s
kubectl -n app-gecko patch service api -p '{"spec":{"selector":{"color":"green"}}}'
