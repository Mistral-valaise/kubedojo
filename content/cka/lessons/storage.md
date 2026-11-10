# Storage

**10% of the exam.** Small domain, very predictable tasks.

## The 20% that solves 80%

### The chain

```text
StorageClass ──provisions──▶ PersistentVolume ◀──binds── PersistentVolumeClaim ◀──uses── Pod
 (cluster)                    (cluster)                    (namespaced)
```

- **Static provisioning:** an admin creates the PV, the claim binds to it.
- **Dynamic provisioning:** the claim names a StorageClass, the provisioner creates the PV.

There are no imperative commands for PV/PVC — copy the YAML from the docs
(*Configure a Pod to Use a PersistentVolume for Storage*).

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata: {name: data, namespace: app}
spec:
  storageClassName: ""            # "" = static, omit = default class, name = that class
  accessModes: [ReadWriteOnce]
  resources: {requests: {storage: 2Gi}}
```

### When does a claim bind?

All of these must match between PVC and PV:

- `storageClassName` (the usual culprit)
- `accessModes` (the PV must offer the requested mode)
- capacity (PV ≥ request)
- optional `selector` / `volumeName`

A `Pending` claim explains itself: `k describe pvc <name>`.

**Trap:** if the cluster has a default StorageClass, a claim *without* `storageClassName` gets a
dynamically provisioned volume instead of your hand-made PV. Use `storageClassName: ""`.

### StorageClass fields worth knowing

| Field | Values | Effect |
|---|---|---|
| `reclaimPolicy` | `Delete` (default), `Retain` | what happens to the PV when the claim is deleted |
| `volumeBindingMode` | `Immediate`, `WaitForFirstConsumer` | bind now, or when a Pod is scheduled |
| `allowVolumeExpansion` | `true` / `false` | may claims be resized |

With `WaitForFirstConsumer` a claim staying `Pending` until a Pod uses it is **normal**, not an error.

The default class is marked by annotation `storageclass.kubernetes.io/is-default-class: "true"`.

### Access modes

`ReadWriteOnce` (one node), `ReadOnlyMany`, `ReadWriteMany`, `ReadWriteOncePod` (one Pod).

### Using a claim in a Pod

```yaml
spec:
  volumes:
    - name: data
      persistentVolumeClaim: {claimName: data}
  containers:
    - name: app
      image: nginx:1.27-alpine
      volumeMounts:
        - {name: data, mountPath: /var/www}
```

### Ephemeral volumes

- `emptyDir` — lives as long as the Pod; the standard way to **share files between containers** of one Pod.
- `configMap`, `secret`, `projected` — configuration as files.
- `hostPath` — a directory of the node; avoid outside of exercises.

## Habits that save points

- `k get pv,pvc -A` first: STATUS and CLAIM columns show the whole binding picture.
- To resize: edit the **claim's** `spec.resources.requests.storage` (the class must allow expansion).
- A PV in `Released` state keeps its old `claimRef` and will not bind again until that is removed.
