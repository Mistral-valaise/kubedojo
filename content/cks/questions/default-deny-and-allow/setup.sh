kubectl delete namespace sec-vault --ignore-not-found --wait >/dev/null
kubectl create namespace sec-vault >/dev/null
mk() { cat <<YAML | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata: {name: $1, namespace: sec-vault}
spec:
  replicas: 1
  selector: {matchLabels: {app: $1}}
  template:
    metadata: {labels: {app: $1}}
    spec:
      terminationGracePeriodSeconds: 1
      containers:
        - name: app
          image: busybox:1.36
          command: ["sh", "-c", "$2"]
YAML
}
mk vault 'while true; do nc -l -p 8200 -e /bin/true; done'
mk client 'sleep 1d'
mk batch 'sleep 1d'
kubectl -n sec-vault expose deployment vault --port=8200 >/dev/null
kubectl -n sec-vault rollout status deploy/vault deploy/client deploy/batch --timeout=180s >/dev/null 2>&1 || true
