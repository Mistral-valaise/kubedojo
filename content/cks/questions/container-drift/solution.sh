set -e
pod=""
for p in $(kubectl -n sec-dunlin get pod -l app=ledger -o jsonpath='{.items[*].metadata.name}'); do
  kubectl -n sec-dunlin exec "$p" -- sh -c 'ls /bin | grep -qx kthrotld' && pod=$p
done
[ -n "$pod" ]
echo "$pod" > /opt/course/drift/pod
kubectl -n sec-dunlin delete pod "$pod" --wait=false
kubectl -n sec-dunlin patch deployment ledger --type=json -p '[
  {"op":"add","path":"/spec/template/spec/volumes","value":[{"name":"data","emptyDir":{}}]},
  {"op":"add","path":"/spec/template/spec/containers/0/volumeMounts","value":[{"name":"data","mountPath":"/data"}]},
  {"op":"add","path":"/spec/template/spec/containers/0/securityContext","value":{"readOnlyRootFilesystem":true}}]'
kubectl -n sec-dunlin rollout status deployment ledger --timeout=180s
