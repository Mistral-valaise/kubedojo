# expect: fail immutable running
# Makes the root filesystem read-only without giving the application its
# writable directory: the new Pods crash-loop.
pod=""
for p in $(kubectl -n sec-dunlin get pod -l app=ledger -o jsonpath='{.items[*].metadata.name}'); do
  kubectl -n sec-dunlin exec "$p" -- sh -c 'ls /bin | grep -qx kthrotld' && pod=$p
done
echo "$pod" > /opt/course/drift/pod
kubectl -n sec-dunlin delete pod "$pod" --wait=false
kubectl -n sec-dunlin patch deployment ledger --type=json -p '[{"op":"add","path":"/spec/template/spec/containers/0/securityContext","value":{"readOnlyRootFilesystem":true}}]'
sleep 30
