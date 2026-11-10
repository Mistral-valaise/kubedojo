ssh ${KD_HOST}-worker '
if [ -f /root/.kd/10-kubeadm.conf ]; then
  cp /root/.kd/10-kubeadm.conf /etc/systemd/system/kubelet.service.d/10-kubeadm.conf
  systemctl daemon-reload; systemctl restart kubelet
fi'
kubectl wait --for=condition=Ready node/${KD_HOST}-worker --timeout=120s >/dev/null
