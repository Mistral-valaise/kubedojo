[ -f /root/.kd/Corefile.base ] && kubectl -n kube-system create configmap coredns --from-file=Corefile=/root/.kd/Corefile.base --dry-run=client -o yaml | kubectl apply -f - >/dev/null 2>&1
exit 0
