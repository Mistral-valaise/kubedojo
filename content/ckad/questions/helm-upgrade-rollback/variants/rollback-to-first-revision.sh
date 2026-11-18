# expect: fail rollout values
# Revision 1 works too, but it is not the last working one: it runs 2 replicas.
# Rolling back to it silently loses the scaling that revision 2 introduced.
set -e
echo "nginx:1.72-alpine" > /opt/course/hare/bad-image.txt
helm -n app-hare rollback ledger 1
helm -n app-hare upgrade ledger /opt/course/hare/ledger --reuse-values --set config.logLevel=warn
kubectl -n app-hare rollout status deployment/ledger --timeout=180s
