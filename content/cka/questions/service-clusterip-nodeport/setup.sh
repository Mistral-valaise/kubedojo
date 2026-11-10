kubectl delete namespace team-gull --ignore-not-found --wait >/dev/null
kubectl create namespace team-gull >/dev/null
kubectl -n team-gull create deployment catalog --image=nginx:1.27-alpine --replicas=2 --port=80 >/dev/null
kubectl -n team-gull rollout status deployment catalog --timeout=120s >/dev/null 2>&1 || true
