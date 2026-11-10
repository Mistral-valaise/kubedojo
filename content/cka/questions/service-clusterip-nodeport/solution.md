```bash
ssh {{host}}
k -n team-gull expose deploy catalog --name catalog --port 8080 --target-port 80

k -n team-gull expose deploy catalog --name catalog-external --type NodePort --port 80 $do > np.yaml
vim np.yaml
```

Add the fixed node port:

```yaml
spec:
  type: NodePort
  ports:
  - port: 80
    protocol: TCP
    targetPort: 80
    nodePort: 30080
  selector:
    app: catalog
```

```bash
k apply -f np.yaml
k -n team-gull get svc,endpointslices
k get nodes -o wide                       # pick any INTERNAL-IP
curl -s <node-ip>:30080 | head -4
```

The three port fields:

| Field | Where it listens |
|---|---|
| `nodePort` | on every node's IP |
| `port` | on the Service's ClusterIP |
| `targetPort` | in the container |
