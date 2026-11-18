# expect: fail checkout-reaches template-labels
# The ledger would accept the connection, but default-deny still blocks checkout's egress.
set -e
kubectl -n app-tiger patch deployment checkout --type=merge \
  -p '{"spec":{"template":{"metadata":{"labels":{"ledger-access":"granted"}}}}}'
kubectl -n app-tiger rollout status deployment checkout --timeout=120s
kubectl -n app-tiger wait --for=delete pod -l 'app=checkout,!ledger-access' --timeout=60s
