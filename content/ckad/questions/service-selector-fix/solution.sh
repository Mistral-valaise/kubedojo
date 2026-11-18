set -e
kubectl -n app-skunk set selector service orders 'app=orders,tier=api'
kubectl -n app-skunk rollout status deploy/api --timeout=120s
kubectl -n app-skunk rollout status deploy/worker --timeout=120s
