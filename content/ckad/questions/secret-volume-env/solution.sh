set -e
kubectl -n app-panda create secret generic ledger-credentials \
  --from-literal=db-password=pa55-panda-7731 \
  --from-literal=api-token=tok-panda-9f2c41
kubectl -n app-panda set env deployment ledger API_TOKEN-
kubectl -n app-panda patch deployment ledger --type strategic -p '
spec:
  template:
    spec:
      volumes:
        - name: token
          secret:
            secretName: ledger-credentials
            defaultMode: 256
            items: [{key: api-token, path: token}]
      containers:
        - name: ledger
          env:
            - name: DB_PASSWORD
              value: null
              valueFrom: {secretKeyRef: {name: ledger-credentials, key: db-password}}
          volumeMounts:
            - {name: token, mountPath: /etc/ledger, readOnly: true}
'
kubectl -n app-panda rollout status deployment ledger --timeout=120s
