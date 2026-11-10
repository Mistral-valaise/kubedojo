mkdir -p /root/.kd /opt/course/dns && rm -f /opt/course/dns/Corefile.bak && chown -R candidate:candidate /opt/course/dns
[ -f /root/.kd/Corefile.base ] || kubectl -n kube-system get cm coredns -o jsonpath='{.data.Corefile}' > /root/.kd/Corefile.base
kubectl -n kube-system create configmap coredns --from-file=Corefile=/root/.kd/Corefile.base --dry-run=client -o yaml | kubectl apply -f - >/dev/null 2>&1
ip=$(kubectl -n kube-system get svc kube-dns -o jsonpath='{.spec.clusterIP}')
for i in $(seq 1 40); do
  [ -z "$(dig +short +time=1 +tries=1 @$ip legacy-db.corp.internal 2>/dev/null)" ] && break; sleep 3
done
exit 0
