# Services & Networking

**20% of the exam.** Services, DNS, NetworkPolicies, Ingress and the Gateway API.

## The 20% that solves 80%

### Services

A Service is a stable virtual IP + DNS name in front of Pods selected **by label**.

```bash
k expose deploy web --name web --port 80 --target-port 8080            # ClusterIP
k expose deploy web --name web-np --type NodePort --port 80
k create service clusterip web --tcp=80:8080 $do
```

| Type | Reachable from | Note |
|---|---|---|
| `ClusterIP` | inside the cluster | default |
| `NodePort` | `<any node IP>:30000-32767` | builds on ClusterIP |
| `LoadBalancer` | external LB | builds on NodePort |
| Headless (`clusterIP: None`) | DNS returns Pod IPs | used by StatefulSets |

`port` is what clients use, `targetPort` is the container port. No endpoints = selector mismatch.

### DNS

```text
<service>.<namespace>.svc.cluster.local
<pod-name>.<headless-service>.<namespace>.svc.cluster.local   # StatefulSet Pods
```

Inside the same Namespace the short name works; across Namespaces use `<service>.<namespace>`.
CoreDNS runs in `kube-system`, configured by ConfigMap `coredns` (the *Corefile*). After editing it:
`k -n kube-system rollout restart deploy coredns`.

### NetworkPolicy — the three rules

1. Without any policy, **everything is allowed**.
2. As soon as a policy selects a Pod for a direction (Ingress/Egress), that direction becomes
   **deny-all except what is listed**.
3. Policies are additive. There are no deny rules.

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: db-access, namespace: shop}
spec:
  podSelector: {matchLabels: {app: db}}       # who is protected
  policyTypes: [Ingress, Egress]
  ingress:
    - from:
        - podSelector: {matchLabels: {app: api}}          # same Namespace
        - namespaceSelector:                               # OR any Pod in Namespace "ops"
            matchLabels: {kubernetes.io/metadata.name: ops}
      ports: [{protocol: TCP, port: 5432}]
  egress:
    - ports: [{protocol: UDP, port: 53}, {protocol: TCP, port: 53}]   # keep DNS working
```

The classic trap: two list items under `from` are **OR**; `podSelector` and `namespaceSelector` in the
**same** item are **AND**. Count the dashes.

Restricting egress without allowing port 53 breaks name resolution — every time.

### Ingress and Gateway API

```bash
k create ingress shop --class=nginx --rule="shop.example.com/api*=api:8080" $do
```

Ingress = one object. Gateway API splits responsibilities:

- `GatewayClass` — which controller (cluster admin)
- `Gateway` — listeners: ports, hostnames, TLS (platform team)
- `HTTPRoute` — routing rules, attached via `parentRefs` (app team)

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata: {name: shop, namespace: shop}
spec:
  parentRefs: [{name: main, namespace: gateway}]
  hostnames: [shop.example.com]
  rules:
    - matches: [{path: {type: PathPrefix, value: /api}}]
      backendRefs: [{name: api, port: 8080}]
```

Migrating an Ingress means: host → `hostnames`, path → `matches`, backend → `backendRefs`.

## Habits that save points

- Test connectivity with a throwaway Pod carrying the right labels:
  `k -n shop run t --rm -i --restart=Never --labels app=api --image=busybox:1.36 -- nc -zv -w 2 db 5432`
- `k explain networkpolicy.spec.ingress.from` is faster than searching the docs.
