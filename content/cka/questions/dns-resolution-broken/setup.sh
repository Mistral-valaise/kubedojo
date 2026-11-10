mkdir -p /root/.kd
[ -f /root/.kd/Corefile ] || kubectl -n kube-system get cm coredns -o jsonpath='{.data.Corefile}' > /root/.kd/Corefile
sed 's/kubernetes cluster\.local/kubernetes cluster.lokal/' /root/.kd/Corefile > /root/.kd/Corefile.broken
kubectl -n kube-system create configmap coredns --from-file=Corefile=/root/.kd/Corefile.broken --dry-run=client -o yaml | kubectl apply -f - >/dev/null 2>&1
ip=$(kubectl -n kube-system get svc kube-dns -o jsonpath='{.spec.clusterIP}')
for i in $(seq 1 40); do
  [ -z "$(dig +short +time=1 +tries=1 @$ip kubernetes.default.svc.cluster.local 2>/dev/null | grep -E '^[0-9.]+$')" ] && break; sleep 3
done
exit 0
