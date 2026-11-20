ssh ${KD_HOST}-worker '
sed -i "/^seccompDefault:/d" /var/lib/kubelet/config.yaml
systemctl restart kubelet'
for i in $(seq 1 60); do
  [ "$(kubectl get node ${KD_HOST}-worker -o jsonpath="{.status.conditions[?(@.type==\"Ready\")].status}")" = "True" ] && break; sleep 2
done
kubectl delete namespace sec-avocet --ignore-not-found --wait >/dev/null
kubectl create namespace sec-avocet >/dev/null
cat <<YAML | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata: {name: reports, namespace: sec-avocet}
spec:
  replicas: 1
  selector: {matchLabels: {app: reports}}
  template:
    metadata: {labels: {app: reports}}
    spec:
      nodeName: ${KD_HOST}-worker
      terminationGracePeriodSeconds: 1
      containers:
        - name: reports
          image: busybox:1.36
          command: ["sleep", "infinity"]
YAML
kubectl -n sec-avocet rollout status deployment reports --timeout=120s >/dev/null 2>&1 || true
exit 0
