# expect: fail spec effective serves
# Drops everything: the chown at startup fails and the Pod crash-loops.
kubectl -n sec-egret patch deployment gateway --type=json -p '[{"op":"replace","path":"/spec/template/spec/containers/0/securityContext","value":{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]}}}]'
sleep 20
