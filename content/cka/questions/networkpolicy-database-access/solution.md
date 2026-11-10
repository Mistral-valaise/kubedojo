```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: db-access
  namespace: team-ibex
spec:
  podSelector:
    matchLabels:
      app: db                       # who is protected
  policyTypes:
    - Ingress
  ingress:
    - from:
        - podSelector:              # source 1: api Pods in this Namespace
            matchLabels:
              app: api
        - namespaceSelector:        # source 2: any Pod in the monitoring Namespace
            matchLabels:
              kubernetes.io/metadata.name: team-ibex-monitoring
      ports:
        - protocol: TCP
          port: 5432
```

```bash
ssh {{host}}
k apply -f netpol.yaml
k -n team-ibex exec deploy/api -- nc -z -w 3 db 5432 && echo open                          # open
k -n team-ibex exec deploy/web -- nc -z -w 3 db 5432 || echo blocked                       # blocked
k -n team-ibex-monitoring exec deploy/probe -- nc -z -w 3 db.team-ibex 5432 && echo open   # open
```

**Count the dashes.** This is the mistake that costs the most points with NetworkPolicies:

```yaml
- from:                      # OR: two list items
    - podSelector: {...}
    - namespaceSelector: {...}

- from:                      # AND: one list item with two keys
    - podSelector: {...}
      namespaceSelector: {...}
```

The second form would mean "Pods labeled `app=api` **inside** the monitoring Namespace".
