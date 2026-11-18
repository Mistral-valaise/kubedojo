kubectl delete namespace app-tiger --ignore-not-found --wait >/dev/null
kubectl create namespace app-tiger >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata: {name: ledger, namespace: app-tiger}
spec:
  replicas: 2
  selector: {matchLabels: {app: ledger}}
  template:
    metadata: {labels: {app: ledger}}
    spec:
      containers:
        - {name: nginx, image: "nginx:1.27-alpine", ports: [{containerPort: 80}]}
---
apiVersion: v1
kind: Service
metadata: {name: ledger, namespace: app-tiger}
spec:
  selector: {app: ledger}
  ports: [{port: 80, targetPort: 80}]
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: default-deny, namespace: app-tiger}
spec:
  podSelector: {}
  policyTypes: [Ingress, Egress]
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: allow-dns, namespace: app-tiger}
spec:
  podSelector: {}
  policyTypes: [Egress]
  egress:
    - ports:
        - {protocol: UDP, port: 53}
        - {protocol: TCP, port: 53}
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: ledger-ingress, namespace: app-tiger}
spec:
  podSelector: {matchLabels: {app: ledger}}
  policyTypes: [Ingress]
  ingress:
    - from:
        - podSelector: {matchLabels: {ledger-access: granted}}
      ports:
        - {protocol: TCP, port: 80}
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: ledger-client-egress, namespace: app-tiger}
spec:
  podSelector: {matchLabels: {role: ledger-client}}
  policyTypes: [Egress]
  egress:
    - to:
        - podSelector: {matchLabels: {app: ledger}}
      ports:
        - {protocol: TCP, port: 80}
YAML
mk() { cat <<YAML | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata: {name: $1, namespace: app-tiger}
spec:
  replicas: 1
  selector: {matchLabels: {app: $1}}
  template:
    metadata: {labels: {app: $1}}
    spec:
      terminationGracePeriodSeconds: 1
      containers:
        - {name: app, image: "busybox:1.36", command: ["sleep", "1d"]}
YAML
}
mk checkout; mk reports
kubectl -n app-tiger rollout status deploy/ledger --timeout=180s >/dev/null 2>&1 || true
kubectl -n app-tiger rollout status deploy/checkout --timeout=180s >/dev/null 2>&1 || true
kubectl -n app-tiger rollout status deploy/reports --timeout=180s >/dev/null 2>&1 || true
