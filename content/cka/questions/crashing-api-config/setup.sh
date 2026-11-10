kubectl delete namespace team-raven --ignore-not-found --wait >/dev/null
kubectl create namespace team-raven >/dev/null
kubectl -n team-raven create secret generic api-credentials --from-literal=token=k8s-dojo-4821 >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: team-raven
spec:
  replicas: 2
  selector:
    matchLabels: {app: api}
  template:
    metadata:
      labels: {app: api}
    spec:
      containers:
        - name: api
          image: nginx:1.27-alpine
          env:
            - name: API_TOKEN
              valueFrom:
                secretKeyRef:
                  name: api-credentials
                  key: api-token
          livenessProbe:
            httpGet: {path: /, port: 8080}
            initialDelaySeconds: 2
            periodSeconds: 3
            failureThreshold: 2
YAML
