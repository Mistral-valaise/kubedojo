`kubectl create ingress` can express everything that is asked for; the rule syntax is
`host/path=service:port[,tls=secret]`, and a trailing `*` turns the path into a `Prefix` path
(without it the path type is `Exact`).

```bash
ssh {{host}}
k get ingressclass                                   # walrus-edge
k -n app-walrus get svc,secret                       # Service ports, the TLS Secret
k -n app-walrus create ingress storefront --class=walrus-edge \
  --rule="store.walrus.example/healthz=storefront-api:9090,tls=storefront-tls" \
  --rule="store.walrus.example/*=storefront:8080,tls=storefront-tls" \
  --default-backend=maintenance:80
k -n app-walrus describe ingress storefront
```

The resulting object, which you can also write by hand (copy the skeleton from the Ingress page of
the docs):

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: storefront
  namespace: app-walrus
spec:
  ingressClassName: walrus-edge
  defaultBackend:                       # requests that match no rule, e.g. another Host header
    service:
      name: maintenance
      port: {number: 80}
  tls:
    - hosts: [store.walrus.example]
      secretName: storefront-tls        # a kubernetes.io/tls Secret in the same Namespace
  rules:
    - host: store.walrus.example
      http:
        paths:
          - path: /healthz
            pathType: Exact             # only /healthz, not /healthz/x
            backend:
              service:
                name: storefront-api
                port: {number: 9090}
          - path: /
            pathType: Prefix            # everything else for this host
            backend:
              service:
                name: storefront
                port: {number: 8080}
```

`k -n app-walrus describe ingress storefront` shows the routing table, the TLS line and the default
backend in one view; a backend that says `<error: services "x" not found>` is a typo in a Service name.

**Takeaway:** the backend port is the **Service** port (8080, 9090), not the container port (80).
`tls` only says which Secret holds the certificate for which host; it does not change the rules. The
`defaultBackend` sits next to `rules`, not inside them. No ADDRESS appears here because no controller
implements the class `walrus-edge`; the object is still valid.
