```bash
ssh {{host}}
k -n team-lark create configmap app-config \
  --from-literal=LOG_LEVEL=debug \
  --from-literal=app.properties=mode=production
k -n team-lark create secret generic app-secret --from-literal=DB_PASSWORD=s3cr3t-lark
k -n team-lark edit deploy app
```

```yaml
    spec:
      volumes:
        - name: config
          configMap:
            name: app-config
            items:
              - key: app.properties
                path: app.properties
      containers:
        - name: app
          image: busybox:1.36
          command: ["sleep", "1d"]
          env:
            - name: LOG_LEVEL
              valueFrom:
                configMapKeyRef:
                  name: app-config
                  key: LOG_LEVEL
          envFrom:
            - secretRef:
                name: app-secret
          volumeMounts:
            - name: config
              mountPath: /etc/app
```

```bash
k -n team-lark rollout status deploy app
k -n team-lark exec deploy/app -- sh -c 'env | grep -E "LOG_LEVEL|DB_PASSWORD"; cat /etc/app/app.properties'
```

**Good to know:** environment variables are read once at container start — a changed ConfigMap needs a
`rollout restart`. Mounted files are updated in place after a short delay (unless mounted with `subPath`).
