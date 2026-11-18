```bash
ssh {{host}}
k -n app-panda set env deploy/ledger --list
# DB_HOST=db.app-panda.svc
# DB_USER=ledger_app
# DB_PASSWORD=pa55-panda-7731
# API_TOKEN=tok-panda-9f2c41
k -n app-panda create secret generic ledger-credentials \
  --from-literal=db-password=pa55-panda-7731 \
  --from-literal=api-token=tok-panda-9f2c41
k -n app-panda edit deploy ledger
```

```yaml
    spec:
      volumes:
        - name: token
          secret:
            secretName: ledger-credentials
            defaultMode: 0400
            items:
              - key: api-token
                path: token                 # file name inside the mount
      containers:
        - name: ledger
          env:
            - {name: DB_HOST, value: db.app-panda.svc}
            - {name: DB_USER, value: ledger_app}
            - name: DB_PASSWORD             # `value:` replaced by `valueFrom:`
              valueFrom:
                secretKeyRef:
                  name: ledger-credentials
                  key: db-password
            # API_TOKEN removed
          volumeMounts:
            - name: token
              mountPath: /etc/ledger
              readOnly: true
```

An env entry may have `value` **or** `valueFrom`, never both: delete the `value:` line.

```bash
k -n app-panda rollout status deploy ledger
k -n app-panda exec deploy/ledger -- ls -lL /etc/ledger          # -r--------  token
k -n app-panda exec deploy/ledger -- sh -c 'cat /etc/ledger/token; echo; echo $DB_PASSWORD; echo ${API_TOKEN-unset}'
k -n app-panda get deploy ledger -o yaml | grep -E 'pa55|tok-'   # nothing
```

**Takeaways**

- `secretKeyRef` for a single variable, a `secret` volume for files; `items` selects keys and names
  the files, otherwise every key becomes a file named after the key.
- `defaultMode: 0400` is octal in YAML and shows up as `256` in `-o yaml`/JSON. In JSON patches write
  `256`.
- Secret volumes are always mounted read-only by the kubelet; `readOnly: true` documents the intent.
- The values in `.data` are base64, not encrypted: `k get secret ledger-credentials -o jsonpath='{.data.api-token}' | base64 -d`.
