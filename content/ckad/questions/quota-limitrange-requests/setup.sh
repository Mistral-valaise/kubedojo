kubectl delete namespace app-rabbit --ignore-not-found --wait >/dev/null
kubectl create namespace app-rabbit >/dev/null
mkdir -p /opt/course/rabbit && rm -rf /opt/course/rabbit/* && chown -R candidate:candidate /opt/course/rabbit
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: v1
kind: ResourceQuota
metadata: {name: rabbit-quota, namespace: app-rabbit}
spec:
  hard:
    requests.cpu: "1"
    requests.memory: 1Gi
    limits.cpu: "2"
    limits.memory: 2Gi
---
apiVersion: v1
kind: LimitRange
metadata: {name: rabbit-limits, namespace: app-rabbit}
spec:
  limits:
    - type: Container
      min: {cpu: 100m, memory: 32Mi}
      max: {cpu: 500m, memory: 512Mi}
      default: {cpu: 500m, memory: 256Mi}
      defaultRequest: {cpu: 500m, memory: 128Mi}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: db, namespace: app-rabbit}
spec:
  replicas: 1
  selector: {matchLabels: {app: db}}
  template:
    metadata: {labels: {app: db}}
    spec:
      terminationGracePeriodSeconds: 1
      containers:
        - name: db
          image: busybox:1.36
          command: ["sleep", "1d"]
          resources:
            requests: {cpu: 250m, memory: 128Mi}
            limits: {cpu: 500m, memory: 256Mi}
YAML
# The admission plugins read quota and LimitRange from a cache: wait until the quota is tracked.
for i in $(seq 1 30); do
  [ -n "$(kubectl -n app-rabbit get resourcequota rabbit-quota -o jsonpath='{.status.hard.requests\.cpu}' 2>/dev/null)" ] && break; sleep 1
done
kubectl -n app-rabbit rollout status deployment db --timeout=180s >/dev/null 2>&1 || true
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata: {name: worker, namespace: app-rabbit}
spec:
  replicas: 3
  selector: {matchLabels: {app: worker}}
  template:
    metadata: {labels: {app: worker}}
    spec:
      terminationGracePeriodSeconds: 1
      containers:
        - name: worker
          image: busybox:1.36
          command: ["sleep", "1d"]
          resources:
            requests: {cpu: 400m, memory: 64Mi}
            limits: {cpu: 800m, memory: 128Mi}
YAML
# Wait until the ReplicaSet reports the rejection. Should a Pod have slipped in before the
# LimitRange was known to the API server, remove it so that the starting state is always "no Pods".
for i in $(seq 1 60); do
  if [ -n "$(kubectl -n app-rabbit get pod -l app=worker -o name 2>/dev/null)" ]; then
    kubectl -n app-rabbit delete pod -l app=worker --wait=false >/dev/null 2>&1
  elif [ -n "$(kubectl -n app-rabbit get events --field-selector reason=FailedCreate -o name 2>/dev/null)" ]; then
    break
  fi
  sleep 2
done
