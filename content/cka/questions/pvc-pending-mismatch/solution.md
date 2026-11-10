```bash
ssh {{host}}
k -n team-osprey describe pvc ledger-data | tail -3
# no persistent volumes available for this claim and no storage class is set
k get pv osprey-pv
# osprey-pv   5Gi   RWO   Retain   Available
k -n team-osprey get pvc ledger-data
# ledger-data   Pending   ...   10Gi requested, RWX
```

Two mismatches: the claim wants **more** than the volume has (10Gi > 5Gi) and an access mode the volume
does not offer (RWX vs. RWO). Both fields are immutable, so the claim has to be recreated.

A claim in use is protected by a finalizer — free it first:

```bash
k -n team-osprey scale deploy ledger --replicas=0
k -n team-osprey get pvc ledger-data -o yaml > pvc.yaml
k -n team-osprey delete pvc ledger-data
vim pvc.yaml          # accessModes: [ReadWriteOnce], storage: 5Gi; remove status, uid, resourceVersion ...
k apply -f pvc.yaml
k -n team-osprey get pvc          # Bound to osprey-pv
k -n team-osprey scale deploy ledger --replicas=1
```

**Binding rules:** same `storageClassName`, PV offers the requested access mode, PV capacity ≥ request.
