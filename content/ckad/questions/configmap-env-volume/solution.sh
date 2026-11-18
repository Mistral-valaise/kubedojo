set -e
kubectl -n app-ocelot create configmap gateway-config \
  --from-literal=UPSTREAM_HOST=orders.app-ocelot.svc \
  --from-literal=UPSTREAM_TIMEOUT=15 \
  --from-literal=FEATURE_BETA=off \
  --from-file=/opt/course/ocelot/gateway.conf
kubectl -n app-ocelot patch deployment gateway --type strategic -p '
spec:
  template:
    spec:
      volumes:
        - name: site
          configMap:
            name: gateway-config
            items: [{key: gateway.conf, path: default.conf}]
      containers:
        - name: nginx
          env:
            - name: UPSTREAM_HOST
              valueFrom: {configMapKeyRef: {name: gateway-config, key: UPSTREAM_HOST}}
            - name: TIMEOUT_SECONDS
              valueFrom: {configMapKeyRef: {name: gateway-config, key: UPSTREAM_TIMEOUT}}
          volumeMounts:
            - {name: site, mountPath: /etc/nginx/conf.d}
'
kubectl -n app-ocelot rollout status deployment gateway --timeout=120s
