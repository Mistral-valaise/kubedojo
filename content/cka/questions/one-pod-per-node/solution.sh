set -e
cat <<'YAML' | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata: {name: ledger-sync, namespace: team-ruff}
spec:
  replicas: 3
  selector: {matchLabels: {app: ledger-sync}}
  template:
    metadata: {labels: {app: ledger-sync}}
    spec:
      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            - labelSelector: {matchLabels: {app: ledger-sync}}
              topologyKey: kubernetes.io/hostname
      containers:
        - {name: nginx, image: "nginx:1.27-alpine"}
YAML
