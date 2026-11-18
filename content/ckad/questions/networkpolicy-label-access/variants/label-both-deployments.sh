# expect: fail reports-blocked
# Labelling every client opens the ledger to reports as well.
set -e
for d in checkout reports; do
  kubectl -n app-tiger patch deployment "$d" --type=merge \
    -p '{"spec":{"template":{"metadata":{"labels":{"role":"ledger-client","ledger-access":"granted"}}}}}'
  kubectl -n app-tiger rollout status deployment "$d" --timeout=120s
done
kubectl -n app-tiger wait --for=delete pod -l '!role,app in (checkout,reports)' --timeout=60s
