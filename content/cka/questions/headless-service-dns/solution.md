```bash
ssh {{host}}
k -n team-robin get sts kv -o jsonpath='{.spec.serviceName}'     # kv
k -n team-robin get svc                                          # nothing
k -n team-robin get pod --show-labels                            # app=kv
```

```yaml
apiVersion: v1
kind: Service
metadata:
  name: kv                 # must equal the StatefulSet's serviceName
  namespace: team-robin
spec:
  clusterIP: None          # headless
  selector:
    app: kv
  ports:
    - port: 80
```

```bash
k apply -f svc.yaml
k -n team-robin exec kv-0 -- nslookup kv-1.kv.team-robin.svc.cluster.local
echo kv-1.kv.team-robin.svc.cluster.local > /opt/course/robin/fqdn
```

| Query | Answer |
|---|---|
| `kv.team-robin.svc.cluster.local` (headless) | the IPs of **all** ready Pods |
| `kv-1.kv.team-robin.svc.cluster.local` | the IP of Pod `kv-1` |
| a normal Service name | the single ClusterIP |
