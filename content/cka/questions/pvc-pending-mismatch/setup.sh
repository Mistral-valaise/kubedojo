kubectl delete namespace team-osprey --ignore-not-found --wait >/dev/null
kubectl delete pv osprey-pv --ignore-not-found --wait >/dev/null
kubectl create namespace team-osprey >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: v1
kind: PersistentVolume
metadata:
  name: osprey-pv
  annotations: {kubedojo.dev/original: "true"}
spec:
  capacity: {storage: 5Gi}
  accessModes: [ReadWriteOnce]
  hostPath: {path: /var/lib/osprey}
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata: {name: ledger-data, namespace: team-osprey}
spec:
  storageClassName: ""
  accessModes: [ReadWriteMany]
  resources: {requests: {storage: 10Gi}}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: ledger, namespace: team-osprey}
spec:
  replicas: 1
  selector: {matchLabels: {app: ledger}}
  template:
    metadata: {labels: {app: ledger}}
    spec:
      volumes:
        - name: data
          persistentVolumeClaim: {claimName: ledger-data}
      containers:
        - name: ledger
          image: nginx:1.27-alpine
          volumeMounts: [{name: data, mountPath: /var/lib/ledger}]
YAML
