# Cluster Hardening

**15% of the exam.** Who may do what — users, ServiceAccounts and the API server itself.

## The 20% that solves 80%

### RBAC: find and remove excess

```bash
k auth can-i --list --as system:serviceaccount:ns:name -n ns
k get clusterrolebinding -o wide | grep -i <name>
k get rolebinding -A -o wide | grep -i <name>
```

Red flags: bindings to `cluster-admin`, wildcards (`*`) in verbs or resources, `secrets` with `get`/`list`,
`pods/exec`, `escalate`, `bind`, `impersonate`, and anything bound to `system:authenticated` or
`system:anonymous`.

Fix by replacing the broad binding with a Role that lists exactly the needed verbs and resources, bound
with a RoleBinding in one Namespace. Verify both the allowed and the forbidden case with `auth can-i`.

### ServiceAccounts

- Every Pod gets the `default` ServiceAccount and a token mounted at
  `/var/run/secrets/kubernetes.io/serviceaccount` unless told otherwise.
- Workloads that never call the API should not have a token:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata: {name: web}
automountServiceAccountToken: false
```

  (also available on the Pod spec, where it wins over the ServiceAccount).
- Give each workload its own ServiceAccount; never add permissions to `default`.
- Tokens are short-lived and bound to the Pod (projected volume). `kubectl create token <sa> --duration 10m`
  issues one manually.

### Users and certificates

Kubernetes has no user objects. A user is a certificate whose **CN is the user name** and whose **O
fields are the groups**, signed by the cluster CA.

```bash
openssl genrsa -out dana.key 2048
openssl req -new -key dana.key -subj "/CN=dana/O=auditors" -out dana.csr
```

```yaml
apiVersion: certificates.k8s.io/v1
kind: CertificateSigningRequest
metadata: {name: dana}
spec:
  request: <base64 -w0 of dana.csr>
  signerName: kubernetes.io/kube-apiserver-client
  usages: [client auth]
```

```bash
k certificate approve dana
k get csr dana -o jsonpath='{.status.certificate}' | base64 -d > dana.crt
```

### The API server

All settings are flags in `/etc/kubernetes/manifests/kube-apiserver.yaml`.

| Flag | Secure value |
|---|---|
| `--anonymous-auth` | `false` (when probes allow it) |
| `--authorization-mode` | `Node,RBAC` — never `AlwaysAllow` |
| `--enable-admission-plugins` | includes `NodeRestriction` |
| `--profiling` | `false` |
| `--insecure-port` / `--kubernetes-service-node-port` | not set / `0` |

`NodeRestriction` limits each kubelet to modifying its own Node and the Pods bound to it.

Upgrade clusters promptly: security fixes only land in supported versions.

## Habits that save points

- Change one flag at a time and wait for the API server to come back (`crictl ps`, `k get --raw /readyz`).
- If the API server does not return, the reason is in `crictl logs` or `/var/log/pods/kube-system_kube-apiserver*`.
