kubectl delete namespace sec-ibis --ignore-not-found --wait >/dev/null
kubectl create namespace sec-ibis >/dev/null
kubectl -n sec-ibis create deployment static-site --image=nginx:1.27-alpine >/dev/null
kubectl -n sec-ibis rollout status deployment static-site --timeout=120s >/dev/null 2>&1 || true
