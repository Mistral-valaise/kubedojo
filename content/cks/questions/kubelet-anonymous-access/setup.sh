ssh ${KD_HOST}-worker '
f=/var/lib/kubelet/config.yaml
mkdir -p /root/.kd; [ -f /root/.kd/kubelet-config.yaml ] || cp $f /root/.kd/kubelet-config.yaml
sed -e "/^  anonymous:/{n;s/enabled: false/enabled: true/}" -e "s/^  mode: Webhook/  mode: AlwaysAllow/" /root/.kd/kubelet-config.yaml > $f
systemctl restart kubelet'
for i in $(seq 1 30); do
  [ "$(ssh ${KD_HOST}-worker "curl -sk -o /dev/null -w %{http_code} https://127.0.0.1:10250/pods")" = "200" ] && break; sleep 2
done
exit 0
