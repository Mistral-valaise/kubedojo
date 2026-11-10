Static provisioning has three links: **PV** (the storage) ← **PVC** (the request) ← **Pod** (the consumer).

```yaml
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
  storageClassName: ""        # opt out of the default StorageClass
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 2Gi
```

```bash
k apply -f pv-pvc.yaml
k -n team-heron get pvc        # STATUS must be Bound, VOLUME archive-pv
```

The trap is the default StorageClass (`k get sc` shows it marked `(default)`). A claim without
`storageClassName` is given the default class and a brand-new volume is provisioned dynamically,
leaving `archive-pv` unused. An empty string means "no class": only PVs without a class match.

Now the Deployment:

```bash
k -n team-heron create deployment archive --image=httpd:2.4-alpine $do > deploy.yaml
```

Add to the Pod template:

```yaml
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
```

```bash
k apply -f deploy.yaml
k -n team-heron get pod,pvc
```
