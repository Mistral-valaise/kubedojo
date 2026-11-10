set -e
cat <<'YAML' | kubectl apply -f -
apiVersion: v1
kind: PersistentVolume
metadata:
  name: archive-pv
spec:
  capacity:
    storage: 2Gi
  accessModes: [ReadWriteOnce]
  hostPath:
    path: /var/lib/archive
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: archive-pvc
  namespace: team-heron
spec:
  storageClassName: ""
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 2Gi
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: archive
  namespace: team-heron
spec:
  replicas: 1
  selector:
    matchLabels: {app: archive}
  template:
    metadata:
      labels: {app: archive}
    spec:
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: archive-pvc
      containers:
        - name: httpd
          image: httpd:2.4-alpine
          volumeMounts:
            - name: data
              mountPath: /usr/local/apache2/htdocs
YAML
