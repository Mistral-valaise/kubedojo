kubectl delete namespace sec-swift --ignore-not-found --wait >/dev/null
kubectl create namespace sec-swift >/dev/null
mkdir -p /opt/course/digest && rm -f /opt/course/digest/edge && chown -R candidate:candidate /opt/course/digest
kubectl -n sec-swift create deployment edge --image=nginx:1.27-alpine --replicas=2 >/dev/null
kubectl -n sec-swift rollout status deployment edge --timeout=180s >/dev/null 2>&1 || true
