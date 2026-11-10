```bash
ssh {{host}}
k -n team-teal set resources deploy renderer --requests=cpu=100m
k -n team-teal autoscale deploy renderer --min=2 --max=6 --cpu-percent=60 $do > hpa.yaml
vim hpa.yaml
```

Add the behavior:

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: renderer
  namespace: team-teal
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: renderer
  minReplicas: 2
  maxReplicas: 6
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 60
  behavior:
    scaleDown:
      stabilizationWindowSeconds: 60
```

```bash
k apply -f hpa.yaml
k -n team-teal get hpa
# renderer   Deployment/renderer   cpu: <unknown>/60%   2   6   2
```

`<unknown>` is expected here because no metrics-server is installed. The autoscaler still enforces
`minReplicas`, so the Deployment is scaled from 1 to 2.

**Formula:** `desired = ceil(current replicas × current utilization / target utilization)`.
