set -e
cat <<'YAML' | kubectl apply -f -
apiVersion: v1
kind: PersistentVolumeClaim
metadata: {name: ledger-data, namespace: app-coyote}
spec:
  accessModes: [ReadWriteOnce]
  resources: {requests: {storage: 200Mi}}
YAML
kubectl -n app-coyote patch deploy ledger --type strategic -p '
spec:
  template:
    spec:
      volumes:
        - {name: data, persistentVolumeClaim: {claimName: ledger-data}}
        - {name: scratch, emptyDir: {medium: Memory, sizeLimit: 64Mi}}
      containers:
        - name: ledger
          volumeMounts:
            - {name: data, mountPath: /var/lib/ledger}
            - {name: scratch, mountPath: /var/cache/ledger}
'
kubectl -n app-coyote rollout status deploy/ledger --timeout=180s
kubectl -n app-coyote wait --for=jsonpath='{.status.phase}'=Bound pvc/ledger-data --timeout=60s
