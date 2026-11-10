set -e
cat <<'YAML' | kubectl apply -f -
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: default-deny, namespace: sec-vault}
spec:
  podSelector: {}
  policyTypes: [Ingress, Egress]
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: client-to-vault, namespace: sec-vault}
spec:
  podSelector: {matchLabels: {app: vault}}
  policyTypes: [Ingress]
  ingress:
    - from: [{podSelector: {matchLabels: {app: client}}}]
      ports: [{protocol: TCP, port: 8200}]
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: client-egress, namespace: sec-vault}
spec:
  podSelector: {matchLabels: {app: client}}
  policyTypes: [Egress]
  egress:
    - to: [{podSelector: {matchLabels: {app: vault}}}]
      ports: [{protocol: TCP, port: 8200}]
    - ports: [{protocol: UDP, port: 53}, {protocol: TCP, port: 53}]
YAML
