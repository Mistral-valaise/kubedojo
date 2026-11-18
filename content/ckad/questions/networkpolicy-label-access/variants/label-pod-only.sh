# expect: fail template-labels
# Labels put on the running Pod work until the Deployment replaces it.
set -e
kubectl -n app-tiger label pod -l app=checkout role=ledger-client ledger-access=granted
