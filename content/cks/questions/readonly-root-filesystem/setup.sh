kubectl delete namespace sec-osprey --ignore-not-found --wait >/dev/null
kubectl create namespace sec-osprey >/dev/null
kubectl -n sec-osprey create deployment cache --image=nginx:1.27-alpine >/dev/null
kubectl -n sec-osprey rollout status deployment cache --timeout=120s >/dev/null 2>&1 || true
