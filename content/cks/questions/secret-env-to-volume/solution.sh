set -e
kubectl -n sec-quail create secret generic payments-db --from-literal=password=hunter2-quail
kubectl -n sec-quail set env deployment payments DB_PASSWORD-
kubectl -n sec-quail patch deployment payments --type strategic -p '
spec:
  template:
    spec:
      volumes:
        - name: db
          secret: {secretName: payments-db, defaultMode: 256}
      containers:
        - name: payments
          volumeMounts:
            - {name: db, mountPath: /etc/payments, readOnly: true}
'
kubectl -n sec-quail rollout status deployment payments --timeout=120s
