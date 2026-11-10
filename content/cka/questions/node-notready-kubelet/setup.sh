ssh ${KD_HOST}-worker '
f=/etc/systemd/system/kubelet.service.d/10-kubeadm.conf
mkdir -p /root/.kd; [ -f /root/.kd/10-kubeadm.conf ] || cp $f /root/.kd/10-kubeadm.conf
sed "s#ExecStart=/usr/bin/kubelet#ExecStart=/usr/local/bin/kubelet#" /root/.kd/10-kubeadm.conf > $f
systemctl daemon-reload; systemctl restart kubelet || true'
# Best effort: wait until the control plane notices.
for i in $(seq 1 30); do
  s=$(kubectl get node ${KD_HOST}-worker -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null)
  [ "$s" != "True" ] && break; sleep 3
done
exit 0
