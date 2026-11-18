Look at what the container offers; the port **names** are what the Services will refer to:

```bash
ssh {{host}}
k -n app-weasel get deploy telemetry -o jsonpath='{.spec.template.spec.containers[0].ports}'
#   [{"containerPort":8080,"name":"http",...},{"containerPort":9113,"name":"metrics",...}]
```

`kubectl expose` can generate the skeleton, but it numbers the ports `port-1`, `port-2` and copies
numeric target ports, so generate YAML and edit it:

```bash
k -n app-weasel expose deploy telemetry --name telemetry --port 80 $do > svc.yaml
vim svc.yaml
```

```yaml
apiVersion: v1
kind: Service
metadata:
  name: telemetry
  namespace: app-weasel
spec:
  selector:
    app: telemetry
  ports:
    - name: web             # required as soon as a Service has more than one port
      port: 80              # what clients connect to
      targetPort: http      # the NAME of the container port (8080 today)
    - name: metrics
      port: 9090
      targetPort: metrics
---
apiVersion: v1
kind: Service
metadata:
  name: telemetry-ext
  namespace: app-weasel
spec:
  type: NodePort
  selector:
    app: telemetry
  ports:
    - name: web
      port: 80
      targetPort: http
      nodePort: 31743       # fixed; without it a random port from 30000-32767
```

```bash
k apply -f svc.yaml
k -n app-weasel get svc,endpointslices
ip=$(k -n app-weasel get svc telemetry -o jsonpath='{.spec.clusterIP}')
curl -s $ip:80            # telemetry web ok
curl -s $ip:9090          # telemetry_up 1
curl -s {{worker}}:31743  # telemetry web ok, on every node
```

**Takeaway:** `port` belongs to the Service, `targetPort` to the Pod. When `targetPort` is a name, the
Service resolves it per Pod from `containers[].ports[].name`: the application can move from 8080 to
another port in a new image version and the Service keeps working, even while old and new Pods run
side by side. A name that no container port carries produces a Service without usable endpoints, so
check `k get endpointslices` after creating it.
