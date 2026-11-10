```bash
ssh {{host}}
k get pv kestrel-pv
# kestrel-pv   2Gi   RWO   Retain   Released   team-kestrel/kestrel-data
k get pv kestrel-pv -o yaml | grep -A6 claimRef
#  claimRef:
#    kind: PersistentVolumeClaim
#    name: kestrel-data
#    namespace: team-kestrel
#    uid: 0e6c...            <- the UID of the deleted claim
```

`Retain` means: keep the volume and its data, and wait for an administrator. Kubernetes will not hand a
`Released` volume to a new claim, because the old data is still on it. You release it by removing the
stale reference:

```bash
k patch pv kestrel-pv --type json -p '[{"op":"remove","path":"/spec/claimRef"}]'
k get pv kestrel-pv          # Available
```

(`k edit pv kestrel-pv` and deleting the `claimRef` block works too.)

Now the new claim. The volume has no class, so the claim must explicitly ask for none:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: kestrel-restore
  namespace: team-kestrel
spec:
  storageClassName: ""
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 2Gi
```

```bash
k apply -f pvc.yaml
k -n team-kestrel get pvc    # Bound, VOLUME kestrel-pv
```

**PV phases:** `Available` → `Bound` → (claim deleted) → `Released` → deleted (`Delete`) or kept until
the claimRef is cleared (`Retain`).
