```bash
ssh {{host}}
k get sc
# NAME                 PROVISIONER             RECLAIMPOLICY   VOLUMEBINDINGMODE
# standard (default)   rancher.io/local-path   Delete          WaitForFirstConsumer
```

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: archive-retain            # no is-default-class annotation
provisioner: rancher.io/local-path
reclaimPolicy: Retain
volumeBindingMode: WaitForFirstConsumer
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: reports-data
  namespace: team-pelican
spec:
  storageClassName: archive-retain
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 1Gi
```

```bash
k apply -f storage.yaml
k -n team-pelican get pvc         # Pending — "waiting for first consumer", this is fine
```

The Pod is the first consumer:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: reports
  namespace: team-pelican
spec:
  volumes:
    - name: data
      persistentVolumeClaim:
        claimName: reports-data
  containers:
    - name: reports
      image: nginx:1.27-alpine
      volumeMounts:
        - name: data
          mountPath: /data
```

```bash
k apply -f pod.yaml
k -n team-pelican get pod,pvc
k get pv                          # RECLAIM POLICY Retain
```
