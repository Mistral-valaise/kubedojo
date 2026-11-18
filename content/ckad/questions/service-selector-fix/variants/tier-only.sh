# expect: pass
# The selector does not have to keep app=orders: tier=api alone selects the same Pods.
set -e
kubectl -n app-skunk patch service orders --type=json -p '[{"op":"replace","path":"/spec/selector","value":{"tier":"api"}}]'
