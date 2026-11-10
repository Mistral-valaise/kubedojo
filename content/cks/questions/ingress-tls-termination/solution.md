```bash
ssh {{host}}
k -n sec-finch create secret tls portal-tls \
  --cert=/opt/course/tls/portal.crt --key=/opt/course/tls/portal.key
k -n sec-finch edit ingress portal
```

```yaml
spec:
  ingressClassName: nginx
  tls:
    - hosts:
        - portal.finch.example
      secretName: portal-tls
  rules:
    - host: portal.finch.example
      ...
```

```bash
k -n sec-finch describe ingress portal
# TLS:
#   portal-tls terminates portal.finch.example
```

With a real controller you would verify with
`curl -kv --resolve portal.finch.example:443:<ingress-ip> https://portal.finch.example` and look at the
certificate's subject.

The Secret must live in the **same Namespace** as the Ingress, and the host under `tls` must match the
host of the rule and the certificate's name.
