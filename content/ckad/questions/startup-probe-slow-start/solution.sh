set -e
kubectl -n app-lemur patch deployment ledger --type strategic -p '
spec:
  template:
    spec:
      containers:
        - name: ledger
          startupProbe:
            httpGet: {path: /healthz, port: http}
            periodSeconds: 5
            failureThreshold: 12
'
kubectl -n app-lemur rollout status deployment ledger --timeout=240s
