if [ -f /root/.kd/kube-controller-manager.yaml ]; then
  cp /root/.kd/kube-controller-manager.yaml /root/.kd/tmp.yaml && mv /root/.kd/tmp.yaml /etc/kubernetes/manifests/kube-controller-manager.yaml
fi
for i in $(seq 1 60); do
  [ "$(kubectl -n kube-system get pod -l component=kube-controller-manager -o jsonpath='{.items[0].status.containerStatuses[0].ready}' 2>/dev/null)" = "true" ] && break; sleep 2
done
kubectl delete deployment ghost -n team-stork --ignore-not-found --wait=false >/dev/null 2>&1
exit 0
