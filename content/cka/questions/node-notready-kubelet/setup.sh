ssh ${KD_HOST}-worker '
f=/etc/systemd/system/kubelet.service.d/10-kubeadm.conf
mkdir -p /root/.kd; [ -f /root/.kd/10-kubeadm.conf ] || cp $f /root/.kd/10-kubeadm.conf
# An earlier attempt may have put a kubelet at the wrong path instead of fixing the unit.
rm -f /usr/local/bin/kubelet
sed "s#ExecStart=/usr/bin/kubelet#ExecStart=/usr/local/bin/kubelet#" /root/.kd/10-kubeadm.conf > $f
grep -q "^ExecStart=/usr/local/bin/kubelet " $f || { echo "setup: $f does not start /usr/local/bin/kubelet" >&2; exit 1; }
systemctl daemon-reload; systemctl restart kubelet || true
# A manual restart sets NRestarts to 0. The first automatic restart proves
# that systemd uses the new ExecStart and that it fails.
for i in $(seq 1 40); do
  [ "$(systemctl show kubelet -p NRestarts --value)" -gt 0 ] 2>/dev/null && exit 0; sleep 0.5
done
echo "setup: kubelet.service did not enter its restart loop" >&2; exit 1' || exit 1
# Wait until the control plane notices: the node controller marks the node
# NotReady about 50 seconds after the last heartbeat.
for i in $(seq 1 50); do
  s=$(kubectl get node ${KD_HOST}-worker -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null)
  [ -n "$s" ] && [ "$s" != "True" ] && exit 0; sleep 3
done
# Still Ready. The node controller is part of the controller manager, and in
# an exam another question may have stopped that one first. Then the node
# turns NotReady when the learner has repaired the controller manager, and
# there is nothing more to wait for here.
[ "$(curl -sk -m 5 https://127.0.0.1:10257/healthz)" = ok ] || exit 0
echo "setup: node ${KD_HOST}-worker is still Ready although its kubelet is down" >&2; exit 1
