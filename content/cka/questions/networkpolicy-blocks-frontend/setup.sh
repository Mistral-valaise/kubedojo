kubectl delete namespace team-crane --ignore-not-found --wait >/dev/null
kubectl create namespace team-crane >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata: {name: backend, namespace: team-crane}
spec:
  replicas: 2
  selector: {matchLabels: {app: backend}}
  template:
    metadata: {labels: {app: backend}}
    spec:
      containers:
        - {name: nginx, image: "nginx:1.27-alpine"}
---
apiVersion: v1
kind: Service
metadata: {name: backend, namespace: team-crane}
spec:
  selector: {app: backend}
  ports: [{port: 80, targetPort: 80}]
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: frontend, namespace: team-crane}
spec:
  replicas: 1
  selector: {matchLabels: {app: frontend}}
  template:
    metadata: {labels: {app: frontend}}
    spec:
      terminationGracePeriodSeconds: 1
      containers:
        - {name: app, image: "busybox:1.36", command: ["sleep", "1d"]}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: intruder, namespace: team-crane}
spec:
  replicas: 1
  selector: {matchLabels: {app: intruder}}
  template:
    metadata: {labels: {app: intruder}}
    spec:
      terminationGracePeriodSeconds: 1
      containers:
        - {name: app, image: "busybox:1.36", command: ["sleep", "1d"]}
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: backend-allow, namespace: team-crane}
spec:
  podSelector: {matchLabels: {app: backend}}
  policyTypes: [Ingress]
  ingress:
    - from:
        - podSelector: {matchLabels: {app: front-end}}
      ports:
        - {protocol: TCP, port: 8080}
YAML
kubectl -n team-crane rollout status deploy/backend deploy/frontend deploy/intruder --timeout=180s >/dev/null 2>&1 || true
