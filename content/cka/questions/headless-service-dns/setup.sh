kubectl delete namespace team-robin --ignore-not-found --wait >/dev/null
kubectl create namespace team-robin >/dev/null
mkdir -p /opt/course/robin && rm -f /opt/course/robin/fqdn && chown -R candidate:candidate /opt/course/robin
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: StatefulSet
metadata: {name: kv, namespace: team-robin}
spec:
  serviceName: kv
  replicas: 2
  podManagementPolicy: Parallel
  selector: {matchLabels: {app: kv}}
  template:
    metadata: {labels: {app: kv}}
    spec:
      terminationGracePeriodSeconds: 1
      containers:
        - {name: kv, image: "nginx:1.27-alpine", ports: [{containerPort: 80}]}
YAML
kubectl -n team-robin rollout status statefulset kv --timeout=180s >/dev/null 2>&1 || true
