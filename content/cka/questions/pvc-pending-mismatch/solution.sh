set -e
kubectl -n team-osprey scale deployment ledger --replicas=0
kubectl -n team-osprey wait --for=delete pod -l app=ledger --timeout=60s
kubectl -n team-osprey delete pvc ledger-data --wait
cat <<'YAML' | kubectl apply -f -
apiVersion: v1
kind: PersistentVolumeClaim
metadata: {name: ledger-data, namespace: team-osprey}
spec:
  storageClassName: ""
  accessModes: [ReadWriteOnce]
  resources: {requests: {storage: 5Gi}}
YAML
kubectl -n team-osprey scale deployment ledger --replicas=1
