set -e
ssh ${KD_HOST}-worker '
sed -i "/^seccompDefault:/d" /var/lib/kubelet/config.yaml
echo "seccompDefault: true" >> /var/lib/kubelet/config.yaml
systemctl restart kubelet'
for i in $(seq 1 60); do
  [ "$(kubectl get node ${KD_HOST}-worker -o jsonpath="{.status.conditions[?(@.type==\"Ready\")].status}")" = "True" ] && break; sleep 2
done
sleep 5
kubectl -n sec-avocet rollout restart deployment reports
kubectl -n sec-avocet rollout status deployment reports --timeout=120s
