# expect: fail kubelet node-default
# Confines the one workload instead of fixing the node default.
set -e
kubectl -n sec-avocet patch deployment reports --type=merge -p '{"spec":{"template":{"spec":{"securityContext":{"seccompProfile":{"type":"RuntimeDefault"}}}}}}'
kubectl -n sec-avocet rollout status deployment reports --timeout=120s
