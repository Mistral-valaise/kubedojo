An egress rule can only *allow*. To block a single address, allow everything and exclude it:

```bash
ssh {{host}}
cat <<'EOF' | k apply -f -
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: deny-metadata
  namespace: sec-plover
spec:
  podSelector: {}
  policyTypes: [Egress]
  egress:
    - to:
        - ipBlock:
            cidr: 0.0.0.0/0
            except:
              - 169.254.169.254/32
EOF
```

**Verify**

```bash
k -n sec-plover describe netpol deny-metadata
k -n sec-plover exec deploy/app -- nslookup kubernetes.default.svc.cluster.local   # still works
k -n sec-plover exec deploy/app -- wget -qO- -T 3 http://169.254.169.254/           # times out
```

**Why not a default deny?** `policyTypes: [Egress]` without rules blocks the metadata address too, but
also DNS and every other destination. The task asks for one destination to disappear.

**Remember:** policies add up. If another policy in the Namespace allows `0.0.0.0/0` without the
`except`, the address is reachable again.
