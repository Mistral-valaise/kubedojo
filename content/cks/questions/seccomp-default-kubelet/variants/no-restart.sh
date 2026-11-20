# expect: fail confined
# Configures the kubelet but leaves the running container as it is.
set -e
ssh ${KD_HOST}-worker '
echo "seccompDefault: true" >> /var/lib/kubelet/config.yaml
systemctl restart kubelet'
for i in $(seq 1 60); do
  [ "$(kubectl get node ${KD_HOST}-worker -o jsonpath="{.status.conditions[?(@.type==\"Ready\")].status}")" = "True" ] && break; sleep 2
done
