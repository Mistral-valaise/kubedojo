```bash
ssh {{host}}
k -n team-plover get ingress portal -o yaml      # what has to be replicated
k -n team-plover get gateway                     # main
k explain httproute.spec.rules --recursive | less
```

Translate field by field:

| Ingress | HTTPRoute |
|---|---|
| `ingressClassName` | `parentRefs` → a Gateway (which has a `gatewayClassName`) |
| `rules[].host` | `hostnames[]` |
| `paths[].path` + `pathType: Prefix` | `rules[].matches[].path` with `type: PathPrefix` |
| `backend.service.name/port` | `rules[].backendRefs[]` with `name`, `port` |

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: portal
  namespace: team-plover
spec:
  parentRefs:
    - name: main
  hostnames:
    - portal.plover.example
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /docs
      backendRefs:
        - name: portal-docs
          port: 8080
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: portal-web
          port: 80
```

```bash
k apply -f route.yaml
k -n team-plover get httproute
k -n team-plover delete ingress portal
```

There is no generator for Gateway API objects. The fastest sources are `kubectl explain` and the
*HTTPRoute* page of the Kubernetes docs.
