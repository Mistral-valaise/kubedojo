# Application Environment, Configuration and Security

**25% of the exam.** The largest domain and the most mechanical one: fields you must type from memory.

## The 20% that solves 80%

### ConfigMaps and Secrets: create, then consume

```bash
k create configmap app-config --from-literal=LOG_LEVEL=debug --from-file=/path/app.conf
k create secret generic app-secret --from-literal=password=s3cr3t     # no trailing newline
k get secret app-secret -o jsonpath='{.data.password}' | base64 -d    # base64, not encrypted
```

| Want | Field |
|---|---|
| one key as a variable (name may differ) | `env[].valueFrom.configMapKeyRef` / `secretKeyRef` |
| all keys as variables | `envFrom[].configMapRef` / `secretRef` |
| keys as files | volume `configMap: {name: ...}` / `secret: {secretName: ...}` + `volumeMounts` |
| some keys, other file names | `items: [{key: app.conf, path: default.conf}]` in the volume |
| file permissions | `defaultMode: 0400` (octal in YAML, shown as `256`) |

- Variables are read **once at container start**: changed ConfigMap → `k rollout restart`.
  Mounted files follow changes after a delay, except with `subPath`.
- A missing ConfigMap, Secret or key → `CreateContainerConfigError` (env) or `ContainerCreating` (volume).
- An env entry has `value` **or** `valueFrom`. Remove a variable: `k set env deploy/x NAME-`.

### SecurityContext

```yaml
spec:
  securityContext:                    # Pod level: all containers, plus fsGroup
    runAsUser: 10001
    fsGroup: 10001                    # group owner of mounted volumes
  containers:
    - securityContext:                # container level wins over Pod level
        runAsNonRoot: true            # kubelet refuses to start UID 0
        allowPrivilegeEscalation: false
        readOnlyRootFilesystem: true
        capabilities: {drop: [ALL], add: [NET_BIND_SERVICE]}
```

`capabilities`, `allowPrivilegeEscalation`, `privileged` and `readOnlyRootFilesystem` exist **only on
the container**. Capability names have no `CAP_` prefix. Check the effect, not the YAML:
`k exec deploy/app -- sh -c 'id; grep -E "NoNewPrivs|Cap(Bnd|Eff)" /proc/1/status'`.

### ServiceAccounts and RBAC from the app's side

```bash
k -n app create serviceaccount reader
k -n app create role job-reader --verb=get,list,watch --resource=jobs.batch
k -n app create rolebinding reader-jobs --role=job-reader --serviceaccount=app:reader
k -n app set serviceaccount deploy/api reader            # spec.serviceAccountName, a rollout
k -n app auth can-i list jobs.batch --as system:serviceaccount:app:reader
```

Four places must agree: the account, the Role (right **API group**: `k api-resources`), the binding
and the Pod template. The token is mounted at `/var/run/secrets/kubernetes.io/serviceaccount/token`.

### Requests, limits and QoS

- **requests**: what the scheduler reserves. No node with that much free → `Pending`.
- **limits**: the ceiling. CPU above the limit is throttled; memory above the limit → `OOMKilled`.
- `k set resources deploy/api --requests=cpu=200m,memory=128Mi --limits=cpu=400m,memory=256Mi`

| QoS class | When | Evicted under node pressure |
|---|---|---|
| `Guaranteed` | every container: requests = limits for CPU and memory | last |
| `Burstable` | at least one request or limit, but not Guaranteed | in between |
| `BestEffort` | no requests and no limits at all | first |

`k get pod x -o jsonpath='{.status.qosClass}'`. Setting only limits makes requests equal to them.

### ResourceQuota and LimitRange

- **LimitRange**: per container/Pod. `min`/`max` reject, `default`/`defaultRequest` fill in what is missing.
- **ResourceQuota**: sum of the Namespace. A quota on `requests.cpu` forces every Pod to have a CPU request.
- Pods that break either rule are **never created**: the Deployment shows `0` Pods, nothing is Pending.

```bash
k describe rs -l app=worker | tail -5          # FailedCreate ... forbidden: exceeded quota / maximum cpu usage
k describe quota,limitrange                    # Used vs. Hard, min/max/defaults
```

### Custom resources

A CRD adds a new kind to the API; from then on it behaves like any built-in resource.

```bash
k get crd | grep -i backup                     # which CRDs exist
k api-resources --api-group=stable.example.com # kind, short names, namespaced?
k explain backupschedule.spec                  # the schema, if the CRD publishes one
k get backupschedules -A                       # list, describe, edit, delete as usual
```

To create one, take `apiVersion` (`<group>/<version>`) and `kind` from `k api-resources` and the fields
from `k explain`, or copy an existing object with `-o yaml`. Without a controller nothing acts on it.

## Habits that save points

- After a template change: `k rollout status`, then look **inside** the container (`k exec deploy/x -- env`).
- Generate, do not type: `k create ... $do > x.yaml`, `k set env`, `k set resources`, `k set serviceaccount`.
- Test RBAC with `auth can-i` in both directions: the verb that must work and one that must not.
