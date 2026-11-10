```bash
ssh {{host}}
k -n team-wren create ingress shop --class=nginx \
  --rule="shop.wren.example/api*=shop-api:8080" \
  --rule="shop.wren.example/*=shop:80"
k -n team-wren describe ingress shop
```

The generated object:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: shop
  namespace: team-wren
spec:
  ingressClassName: nginx
  rules:
    - host: shop.wren.example
      http:
        paths:
          - path: /api
            pathType: Prefix
            backend:
              service:
                name: shop-api
                port:
                  number: 8080
          - path: /
            pathType: Prefix
            backend:
              service:
                name: shop
                port:
                  number: 80
```

Details that are easy to get wrong: the trailing `*` in `--rule` (otherwise `pathType: Exact`), the
Service **port** (not the container port), and `ingressClassName` (`k get ingressclass`).
