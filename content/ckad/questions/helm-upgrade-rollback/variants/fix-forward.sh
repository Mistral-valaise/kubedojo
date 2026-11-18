# expect: pass
# Rolling back by hand: a new revision that sets the image tag back. The
# release ends up with the same values as after "rollback 2" plus the upgrade.
set -e
kubectl -n app-hare get pods -o jsonpath='{range .items[*]}{.spec.containers[0].image}{"\n"}{end}' \
  | grep -v '1.27-alpine' | sort -u > /opt/course/hare/bad-image.txt
helm -n app-hare upgrade ledger /opt/course/hare/ledger --reuse-values \
  --set image.tag=1.27-alpine --set config.logLevel=warn
kubectl -n app-hare rollout status deployment/ledger --timeout=180s
