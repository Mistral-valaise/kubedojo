```bash
ssh {{host}}
k -n sec-quail get deploy payments -o yaml | grep -B1 -A1 DB_PASSWORD
#        - name: DB_PASSWORD
#          value: hunter2-quail
k -n sec-quail create secret generic payments-db --from-literal=password=hunter2-quail
k -n sec-quail set env deploy payments DB_PASSWORD-
k -n sec-quail edit deploy payments
```

```yaml
    spec:
      volumes:
        - name: db
          secret:
            secretName: payments-db
            defaultMode: 0400
      containers:
        - name: payments
          volumeMounts:
            - name: db
              mountPath: /etc/payments
              readOnly: true
```

```bash
k -n sec-quail rollout status deploy payments
k -n sec-quail exec deploy/payments -- ls -lL /etc/payments/password      # -r--------
k -n sec-quail exec deploy/payments -- cat /etc/payments/password
```

**Why files instead of env vars:** environment variables are visible in `kubectl describe pod`, are
inherited by every child process, often end up in logs and crash reports, and never change while the
container runs. A mounted Secret is a tmpfs file with its own permissions that follows updates.

The old password was exposed in the Deployment's history — in real life you would rotate it now.
