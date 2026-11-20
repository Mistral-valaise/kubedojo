# expect: fail blocked
# The right policy, next to another one that allows all egress: policies add up.
set -e
cat <<'YAML' | kubectl apply -f -
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
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: allow-all-egress, namespace: sec-plover}
spec:
  podSelector: {}
  policyTypes: [Egress]
  egress:
    - {}
YAML
