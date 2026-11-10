if [ -f /root/.kd/kube-scheduler.yaml ]; then
  cp /root/.kd/kube-scheduler.yaml /root/.kd/tmp.yaml && mv /root/.kd/tmp.yaml /etc/kubernetes/manifests/kube-scheduler.yaml
fi
for i in $(seq 1 60); do
  [ "$(kubectl -n kube-system get pod -l component=kube-scheduler -o jsonpath='{.items[0].status.containerStatuses[0].ready}' 2>/dev/null)" = "true" ] && break; sleep 2
done
kubectl delete pod reports -n team-ibis --ignore-not-found --wait=false >/dev/null 2>&1
exit 0
