set -e
kubectl -n app-tiger patch deployment checkout --type=merge \
  -p '{"spec":{"template":{"metadata":{"labels":{"role":"ledger-client","ledger-access":"granted"}}}}}'
kubectl -n app-tiger rollout status deployment checkout --timeout=120s
kubectl -n app-tiger wait --for=delete pod -l 'app=checkout,!role' --timeout=60s
