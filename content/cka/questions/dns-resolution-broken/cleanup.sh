if [ -f /root/.kd/Corefile ]; then
  kubectl -n kube-system create configmap coredns --from-file=Corefile=/root/.kd/Corefile --dry-run=client -o yaml | kubectl apply -f - >/dev/null 2>&1
fi
ip=$(kubectl -n kube-system get svc kube-dns -o jsonpath='{.spec.clusterIP}')
for i in $(seq 1 40); do
  dig +short +time=1 +tries=1 @$ip kubernetes.default.svc.cluster.local 2>/dev/null | grep -qE '^[0-9.]+$' && break; sleep 3
done
exit 0
