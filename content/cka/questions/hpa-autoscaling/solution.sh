set -e
kubectl -n team-teal set resources deployment renderer --requests=cpu=100m
cat <<'YAML' | kubectl apply -f -
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata: {name: renderer, namespace: team-teal}
spec:
  scaleTargetRef: {apiVersion: apps/v1, kind: Deployment, name: renderer}
  minReplicas: 2
  maxReplicas: 6
  metrics:
    - type: Resource
      resource: {name: cpu, target: {type: Utilization, averageUtilization: 60}}
  behavior:
    scaleDown: {stabilizationWindowSeconds: 60}
YAML
