set -e
helm -n app-hare get values ledger --revision 3 -a -o json \
  | jq -r '"\(.image.repository):\(.image.tag)"' > /opt/course/hare/bad-image.txt
helm -n app-hare rollback ledger 2
helm -n app-hare upgrade ledger /opt/course/hare/ledger --reuse-values --set config.logLevel=warn
kubectl -n app-hare rollout status deployment/ledger --timeout=180s
