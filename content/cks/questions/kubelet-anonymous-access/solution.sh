set -e
ssh ${KD_HOST}-worker '
sed -i -e "/^  anonymous:/{n;s/enabled: true/enabled: false/}" -e "s/^  mode: AlwaysAllow/  mode: Webhook/" /var/lib/kubelet/config.yaml
systemctl restart kubelet'
