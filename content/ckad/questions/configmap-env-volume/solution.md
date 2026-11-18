```bash
ssh {{host}}
k -n app-ocelot create configmap gateway-config \
  --from-literal=UPSTREAM_HOST=orders.app-ocelot.svc \
  --from-literal=UPSTREAM_TIMEOUT=15 \
  --from-literal=FEATURE_BETA=off \
  --from-file=/opt/course/ocelot/gateway.conf
k -n app-ocelot describe cm gateway-config        # four keys, the last one is the whole file
k -n app-ocelot edit deploy gateway
```

```yaml
    spec:
      volumes:
        - name: site
          configMap:
            name: gateway-config
            items:
              - key: gateway.conf
                path: default.conf          # the file name inside the mount
      containers:
        - name: nginx
          image: nginx:1.27-alpine
          env:
            - name: UPSTREAM_HOST
              valueFrom:
                configMapKeyRef: {name: gateway-config, key: UPSTREAM_HOST}
            - name: TIMEOUT_SECONDS         # variable name differs from the key
              valueFrom:
                configMapKeyRef: {name: gateway-config, key: UPSTREAM_TIMEOUT}
          volumeMounts:
            - name: site
              mountPath: /etc/nginx/conf.d
```

Check what the container really sees, not only the YAML:

```bash
k -n app-ocelot rollout status deploy gateway
k -n app-ocelot exec deploy/gateway -- sh -c 'env | grep -E "UPSTREAM|TIMEOUT|FEATURE"; ls /etc/nginx/conf.d'
k -n app-ocelot exec deploy/gateway -- wget -qO- 127.0.0.1:8080/healthz      # ocelot gateway ok
```

**Why `items`:** without it the volume contains one file per key, so `conf.d` would hold
`UPSTREAM_HOST`, `UPSTREAM_TIMEOUT`, `FEATURE_BETA` and `gateway.conf`. `items` selects keys and
renames them. A `subPath: gateway.conf` mount on `/etc/nginx/conf.d/default.conf` also works, but a
`subPath` mount never receives later updates of the ConfigMap.

**Why not `envFrom`:** it imports every key whose name is a valid variable name, here also
`FEATURE_BETA`, and cannot rename `UPSTREAM_TIMEOUT` to `TIMEOUT_SECONDS`.
