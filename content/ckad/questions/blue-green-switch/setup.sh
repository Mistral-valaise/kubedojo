kubectl delete namespace app-gecko --ignore-not-found --wait >/dev/null
kubectl create namespace app-gecko >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-blue
  namespace: app-gecko
spec:
  replicas: 3
  selector:
    matchLabels: {app: api, color: blue}
  template:
    metadata:
      labels: {app: api, color: blue}
    spec:
      terminationGracePeriodSeconds: 2
      containers:
        - name: api
          image: httpd:2.4-alpine
          ports:
            - containerPort: 80
          readinessProbe:
            httpGet: {path: /, port: 80}
            periodSeconds: 2
---
apiVersion: v1
kind: Service
metadata:
  name: api
  namespace: app-gecko
  annotations:
    kubedojo.dev/original: "true"
spec:
  selector: {app: api, color: blue}
  ports:
    - port: 80
      targetPort: 80
YAML
kubectl -n app-gecko rollout status deploy/api-blue --timeout=180s >/dev/null 2>&1 || true
