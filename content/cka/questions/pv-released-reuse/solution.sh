set -e
kubectl patch pv kestrel-pv --type json -p '[{"op":"remove","path":"/spec/claimRef"}]'
cat <<'YAML' | kubectl apply -f -
apiVersion: v1
kind: PersistentVolumeClaim
metadata: {name: kestrel-restore, namespace: team-kestrel}
spec:
  storageClassName: ""
  accessModes: [ReadWriteOnce]
  resources: {requests: {storage: 2Gi}}
YAML
