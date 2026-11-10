set -e
kubectl -n team-lark create configmap app-config --from-literal=LOG_LEVEL=debug --from-literal=app.properties=mode=production
kubectl -n team-lark create secret generic app-secret --from-literal=DB_PASSWORD=s3cr3t-lark
kubectl -n team-lark patch deployment app --type strategic -p '
spec:
  template:
    spec:
      volumes:
        - name: config
          configMap:
            name: app-config
            items: [{key: app.properties, path: app.properties}]
      containers:
        - name: app
          env:
            - name: LOG_LEVEL
              valueFrom: {configMapKeyRef: {name: app-config, key: LOG_LEVEL}}
          envFrom:
            - secretRef: {name: app-secret}
          volumeMounts:
            - {name: config, mountPath: /etc/app}
'
kubectl -n team-lark rollout status deployment app --timeout=120s
