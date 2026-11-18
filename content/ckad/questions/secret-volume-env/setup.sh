kubectl delete namespace app-panda --ignore-not-found --wait >/dev/null
kubectl create namespace app-panda >/dev/null
cat <<'YAML' | kubectl create -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata: {name: ledger, namespace: app-panda}
spec:
  replicas: 1
  selector: {matchLabels: {app: ledger}}
  template:
    metadata: {labels: {app: ledger}}
    spec:
      terminationGracePeriodSeconds: 1
      containers:
        - name: ledger
          image: busybox:1.36
          command: ["sleep", "1d"]
          env:
            - {name: DB_HOST, value: db.app-panda.svc}
            - {name: DB_USER, value: ledger_app}
            - {name: DB_PASSWORD, value: pa55-panda-7731}
            - {name: API_TOKEN, value: tok-panda-9f2c41}
YAML
kubectl -n app-panda rollout status deployment ledger --timeout=180s >/dev/null 2>&1 || true
