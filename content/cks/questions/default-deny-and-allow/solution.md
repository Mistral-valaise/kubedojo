**Step 1 — deny everything.**

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny
  namespace: sec-vault
spec:
  podSelector: {}
  policyTypes: [Ingress, Egress]
```

**Step 2 — allow one connection.** With deny in both directions, traffic from `client` to `vault` must be
allowed twice: leaving the client (egress) and entering the vault (ingress). A policy selects one group of
Pods, so this takes two policies:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: client-to-vault
  namespace: sec-vault
spec:
  podSelector:
    matchLabels: {app: vault}
  policyTypes: [Ingress]
  ingress:
    - from:
        - podSelector:
            matchLabels: {app: client}
      ports:
        - {protocol: TCP, port: 8200}
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: client-egress
  namespace: sec-vault
spec:
  podSelector:
    matchLabels: {app: client}
  policyTypes: [Egress]
  egress:
    - to:
        - podSelector:
            matchLabels: {app: vault}
      ports:
        - {protocol: TCP, port: 8200}
    - ports:                          # DNS, to any destination
        - {protocol: UDP, port: 53}
        - {protocol: TCP, port: 53}
```

```bash
ssh {{host}}
k apply -f policies.yaml
VAULT=$(k -n sec-vault get pod -l app=vault -o jsonpath='{.items[0].status.podIP}')
k -n sec-vault exec deploy/client -- nc -z -w 3 $VAULT 8200 && echo open        # open
k -n sec-vault exec deploy/batch  -- nc -z -w 3 $VAULT 8200 || echo blocked     # blocked
k -n sec-vault exec deploy/client -- nslookup vault.sec-vault.svc.cluster.local # resolves
```

Policies are additive: the allow rules punch holes into the default deny, they never conflict with it.

**Test with Pod IPs.** NetworkPolicies are defined for Pod-to-Pod traffic. Whether an egress rule with a
`podSelector` also matches traffic sent to a Service's ClusterIP depends on the network plugin and the
kube-proxy mode, so a test through the Service name can mislead you in either direction.
