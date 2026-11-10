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
k -n sec-vault exec deploy/client -- nc -z -w 3 vault 8200 && echo open        # open
k -n sec-vault exec deploy/batch  -- nc -z -w 3 vault 8200 || echo blocked     # blocked
```

Policies are additive: the allow rules punch holes into the default deny, they never conflict with it.
If the client could connect by IP but not by name, the DNS rule is what is missing.
