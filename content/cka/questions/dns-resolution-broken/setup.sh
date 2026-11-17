mkdir -p /root/.kd
[ -f /root/.kd/Corefile ] || kubectl -n kube-system get cm coredns -o jsonpath='{.data.Corefile}' > /root/.kd/Corefile
sed 's/kubernetes cluster\.local/kubernetes cluster.lokal/' /root/.kd/Corefile > /root/.kd/Corefile.broken
kubectl -n kube-system create configmap coredns --from-file=Corefile=/root/.kd/Corefile.broken --dry-run=client -o yaml | kubectl apply -f - >/dev/null 2>&1
ip=$(kubectl -n kube-system get svc kube-dns -o jsonpath='{.spec.clusterIP}')
# CoreDNS picks a changed Corefile up only after a minute or two. Restart it so
# the question is broken when it is handed over, and wait until the name is
# gone three times in a row: a single empty answer can also be a timeout.
kubectl -n kube-system rollout restart deployment coredns >/dev/null 2>&1
kubectl -n kube-system rollout status deployment coredns --timeout=120s >/dev/null 2>&1
gone=0
for i in $(seq 1 60); do
  if [ -z "$(dig +short +time=2 +tries=1 @$ip kubernetes.default.svc.cluster.local 2>/dev/null | grep -E '^[0-9.]+$')" ]; then gone=$((gone + 1)); else gone=0; fi
  [ $gone -ge 3 ] && break; sleep 2
done
exit 0
