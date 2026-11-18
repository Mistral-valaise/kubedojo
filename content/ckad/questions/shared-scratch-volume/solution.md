Find out what the container needs:

```bash
ssh {{host}}
k -n app-coyote get pod                        # CrashLoopBackOff
k -n app-coyote logs deploy/ledger
# sh: can't create /var/cache/ledger/heartbeat: nonexistent directory
k get storageclass                             # standard (default), WaitForFirstConsumer
```

The claim, without `storageClassName`, so that the default class is used:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ledger-data
  namespace: app-coyote
spec:
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 200Mi
```

```bash
k apply -f pvc.yaml
k -n app-coyote get pvc                        # Pending: WaitForFirstConsumer, that is expected
k -n app-coyote edit deploy ledger
```

```yaml
    spec:
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: ledger-data
        - name: scratch
          emptyDir:
            medium: Memory                     # tmpfs instead of the node's disk
            sizeLimit: 64Mi
      containers:
        - name: ledger
          volumeMounts:
            - name: data
              mountPath: /var/lib/ledger
            - name: scratch
              mountPath: /var/cache/ledger
```

```bash
k -n app-coyote rollout status deploy ledger
k -n app-coyote get pvc                        # Bound
k -n app-coyote exec deploy/ledger -- sh -c 'mount | grep ledger'
# /dev/...  on /var/lib/ledger   type ext4  (rw,...)
# tmpfs     on /var/cache/ledger type tmpfs (rw,...,size=65536k)
```

**Takeaways:**

- `readOnlyRootFilesystem: true` plus a volume for every path the process writes is the standard
  hardening pattern. Removing the flag is the wrong fix.
- `emptyDir` lives and dies with the Pod (it survives container restarts); a PersistentVolumeClaim
  outlives the Pod. `medium: Memory` counts against the container's memory limit.
- A claim of a `WaitForFirstConsumer` class stays `Pending` until a Pod uses it. Do not wait for
  `Bound` before you edit the Deployment.
- A `ReadWriteOnce` volume can be mounted by several Pods on the **same** node, which is why the
  rolling update works here; across nodes a Deployment with such a claim needs `strategy: Recreate`.
