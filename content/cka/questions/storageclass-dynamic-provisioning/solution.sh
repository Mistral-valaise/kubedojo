set -e
cat <<'YAML' | kubectl apply -f -
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata: {name: archive-retain}
provisioner: rancher.io/local-path
reclaimPolicy: Retain
volumeBindingMode: WaitForFirstConsumer
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata: {name: reports-data, namespace: team-pelican}
spec:
  storageClassName: archive-retain
  accessModes: [ReadWriteOnce]
  resources: {requests: {storage: 1Gi}}
---
apiVersion: v1
kind: Pod
metadata: {name: reports, namespace: team-pelican}
spec:
  volumes:
    - name: data
      persistentVolumeClaim: {claimName: reports-data}
  containers:
    - name: reports
      image: nginx:1.27-alpine
      volumeMounts:
        - {name: data, mountPath: /data}
YAML
