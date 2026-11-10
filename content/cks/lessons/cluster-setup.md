# Cluster Setup

**15% of the exam.** Lock down the network, the node endpoints and the things you install.

## The 20% that solves 80%

### Default deny, then allow

Kubernetes networking is open by default. The baseline for every Namespace is a policy that selects all
Pods and allows nothing:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: default-deny, namespace: shop}
spec:
  podSelector: {}                 # every Pod in the Namespace
  policyTypes: [Ingress, Egress]
```

Then add narrow allow policies. Three things candidates forget:

- **Egress needs DNS.** Allow UDP and TCP port 53, or names stop resolving.
- **Both directions.** With default deny for Ingress *and* Egress, a connection from A to B needs an
  egress rule on A and an ingress rule on B.
- **Dashes.** Two items under `from`/`to` are OR. `podSelector` + `namespaceSelector` in one item are AND.

Cloud metadata endpoints (`169.254.169.254`) are blocked with an egress `ipBlock` and `except`.

### CIS benchmark

`kube-bench` checks a node against the CIS Kubernetes Benchmark and tells you the fix. Know where the
fixes go:

| Finding about | Fix in |
|---|---|
| kube-apiserver, controller-manager, scheduler, etcd flags | `/etc/kubernetes/manifests/*.yaml` |
| kubelet settings | `/var/lib/kubelet/config.yaml`, then `systemctl restart kubelet` |
| file permissions/ownership | `chmod` / `chown` on the named file |

Kubelet essentials (every kubelet has an HTTPS API on port 10250):

```yaml
authentication:
  anonymous:
    enabled: false
  webhook:
    enabled: true
authorization:
  mode: Webhook
```

`curl -sk https://localhost:10250/pods` must answer `401 Unauthorized`.

### TLS for Ingress

```bash
k -n shop create secret tls shop-tls --cert=tls.crt --key=tls.key
```

```yaml
spec:
  tls:
    - hosts: [shop.example.com]
      secretName: shop-tls
```

The Secret must be in the Ingress's Namespace and of type `kubernetes.io/tls`.

### Verify binaries before you trust them

```bash
sha512sum kubelet                               # compare with the published checksum
echo "<checksum>  kubelet" | sha512sum -c -     # or let the tool compare
```

A single different character means a different file.

## Habits that save points

- After editing a static Pod manifest, watch `crictl ps` until the component is back before moving on.
- Keep a copy of any file you edit — outside `/etc/kubernetes/manifests`.
- Test policies from a Pod with the right labels, not from the node.
