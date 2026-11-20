# expect: fail spec effective
# Adds CHOWN but keeps one of the unneeded capabilities.
set -e
kubectl -n sec-egret patch deployment gateway --type=json -p '[{"op":"replace","path":"/spec/template/spec/containers/0/securityContext","value":{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"],"add":["CHOWN","NET_ADMIN"]}}}]'
kubectl -n sec-egret rollout status deployment gateway --timeout=120s
