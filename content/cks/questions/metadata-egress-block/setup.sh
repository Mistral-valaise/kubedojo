kubectl delete namespace sec-plover --ignore-not-found --wait >/dev/null
kubectl create namespace sec-plover >/dev/null
kubectl -n sec-plover create deployment app --image=busybox:1.36 -- sleep infinity >/dev/null
kubectl -n sec-plover rollout status deployment app --timeout=120s >/dev/null 2>&1 || true
