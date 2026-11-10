set -e
kubectl label node ${KD_HOST}-worker2 disk=ssd --overwrite
kubectl -n team-finch set resources deployment indexer --requests=cpu=100m
kubectl -n team-finch rollout status deployment indexer --timeout=120s
