set -e
ssh ${KD_HOST}-worker '
sed -i "s#ExecStart=/usr/local/bin/kubelet#ExecStart=/usr/bin/kubelet#" /etc/systemd/system/kubelet.service.d/10-kubeadm.conf
systemctl daemon-reload && systemctl restart kubelet'
