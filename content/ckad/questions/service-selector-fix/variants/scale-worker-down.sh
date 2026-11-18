# expect: fail workloads-intact
# Getting rid of the worker Pods also empties the endpoint list of everything but the api.
set -e
kubectl -n app-skunk scale deployment worker --replicas=0
kubectl -n app-skunk wait --for=delete pod -l tier=worker --timeout=60s
