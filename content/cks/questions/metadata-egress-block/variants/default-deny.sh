# expect: fail other-egress
# Blocks the metadata address by blocking everything: DNS and the API are gone
# too, and no rule excludes the address explicitly.
set -e
cat <<'YAML' | kubectl apply -f -
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: deny-metadata, namespace: sec-plover}
spec:
  podSelector: {}
  policyTypes: [Egress]
YAML
