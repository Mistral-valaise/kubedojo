kubectl delete namespace team-kestrel --ignore-not-found --wait >/dev/null
kubectl delete pv kestrel-pv --ignore-not-found --wait >/dev/null
kubectl create namespace team-kestrel >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: v1
kind: PersistentVolume
metadata:
  name: kestrel-pv
  annotations: {kubedojo.dev/original: "true"}
spec:
  capacity: {storage: 2Gi}
  accessModes: [ReadWriteOnce]
  persistentVolumeReclaimPolicy: Retain
  hostPath: {path: /var/lib/kestrel}
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata: {name: kestrel-data, namespace: team-kestrel}
spec:
  storageClassName: ""
  volumeName: kestrel-pv
  accessModes: [ReadWriteOnce]
  resources: {requests: {storage: 2Gi}}
YAML
kubectl -n team-kestrel wait --for=jsonpath='{.status.phase}'=Bound pvc/kestrel-data --timeout=60s >/dev/null
kubectl -n team-kestrel delete pvc kestrel-data --wait >/dev/null
kubectl wait --for=jsonpath='{.status.phase}'=Released pv/kestrel-pv --timeout=60s >/dev/null
