kubectl delete namespace team-finch --ignore-not-found --wait >/dev/null
kubectl label node --all disk- >/dev/null 2>&1
kubectl create namespace team-finch >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata:
  name: indexer
  namespace: team-finch
spec:
  replicas: 1
  selector:
    matchLabels: {app: indexer}
  template:
    metadata:
      labels: {app: indexer}
    spec:
      nodeSelector:
        disk: ssd
      containers:
        - name: indexer
          image: busybox:1.36
          command: ["sleep", "1d"]
          resources:
            requests: {cpu: "64", memory: 64Mi}
YAML
