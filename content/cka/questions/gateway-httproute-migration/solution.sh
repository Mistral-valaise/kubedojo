set -e
cat <<'YAML' | kubectl apply -f -
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata: {name: portal, namespace: team-plover}
spec:
  parentRefs: [{name: main}]
  hostnames: [portal.plover.example]
  rules:
    - matches: [{path: {type: PathPrefix, value: /docs}}]
      backendRefs: [{name: portal-docs, port: 8080}]
    - matches: [{path: {type: PathPrefix, value: /}}]
      backendRefs: [{name: portal-web, port: 80}]
YAML
kubectl -n team-plover delete ingress portal
