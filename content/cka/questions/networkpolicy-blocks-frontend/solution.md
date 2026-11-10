Timeouts (not "connection refused") between Pods that both run are the fingerprint of a NetworkPolicy.

```bash
ssh {{host}}
k -n team-crane exec deploy/frontend -- wget -qO- -T 3 backend     # wget: download timed out
k -n team-crane get netpol
k -n team-crane describe netpol backend-allow
#   PodSelector:  app=backend
#   Allowing ingress traffic:
#     To Port: 8080/TCP
#     From:  PodSelector: app=front-end
k -n team-crane get pod --show-labels                               # app=frontend
```

Two mismatches: the label (`front-end` vs. `frontend`) and the port (`8080` vs. `80`).
For NetworkPolicies, `port` is the port of the **destination Pod**, not of the Service.

```bash
k -n team-crane edit netpol backend-allow
```

```yaml
  ingress:
    - from:
        - podSelector:
            matchLabels:
              app: frontend
      ports:
        - protocol: TCP
          port: 80
```

Verify both directions of the requirement:

```bash
k -n team-crane exec deploy/frontend -- wget -qO- -T 3 backend | head -4    # works
k -n team-crane exec deploy/intruder -- wget -qO- -T 3 backend              # still times out
```
