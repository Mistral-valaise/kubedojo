set -e
cat <<'YAML' | kubectl apply -f -
apiVersion: v1
kind: ServiceAccount
metadata: {name: static-site, namespace: sec-ibis}
automountServiceAccountToken: false
YAML
kubectl -n sec-ibis set serviceaccount deployment static-site static-site
kubectl -n sec-ibis rollout status deployment static-site --timeout=120s
