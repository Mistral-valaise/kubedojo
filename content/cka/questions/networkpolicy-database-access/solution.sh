set -e
cat <<'YAML' | kubectl apply -f -
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: db-access, namespace: team-ibex}
spec:
  podSelector: {matchLabels: {app: db}}
  policyTypes: [Ingress]
  ingress:
    - from:
        - podSelector: {matchLabels: {app: api}}
        - namespaceSelector: {matchLabels: {kubernetes.io/metadata.name: team-ibex-monitoring}}
      ports:
        - {protocol: TCP, port: 5432}
YAML
