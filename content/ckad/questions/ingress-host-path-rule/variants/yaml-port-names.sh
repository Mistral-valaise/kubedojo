# expect: pass
# Written as YAML, with the backends referenced by Service port name instead of number.
set -e
cat <<'YAML' | kubectl apply -f -
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata: {name: storefront, namespace: app-walrus}
spec:
  ingressClassName: walrus-edge
  defaultBackend:
    service: {name: maintenance, port: {name: http}}
  tls:
    - hosts: [store.walrus.example]
      secretName: storefront-tls
  rules:
    - host: store.walrus.example
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service: {name: storefront, port: {name: http}}
          - path: /healthz
            pathType: Exact
            backend:
              service: {name: storefront-api, port: {name: http}}
YAML
