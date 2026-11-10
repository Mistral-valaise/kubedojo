f=/etc/kubernetes/manifests/kube-controller-manager.yaml
mkdir -p /root/.kd; [ -f /root/.kd/kube-controller-manager.yaml ] || cp $f /root/.kd/kube-controller-manager.yaml
# Start from a healthy controller manager so the old Deployment can be removed.
if ! cmp -s /root/.kd/kube-controller-manager.yaml $f; then
  cp /root/.kd/kube-controller-manager.yaml /root/.kd/tmp.yaml && mv /root/.kd/tmp.yaml $f
  for i in $(seq 1 60); do
    [ "$(kubectl -n kube-system get pod -l component=kube-controller-manager -o jsonpath='{.items[0].status.containerStatuses[0].ready}' 2>/dev/null)" = "true" ] && break; sleep 2
  done
fi
kubectl delete deployment ghost -n team-stork --ignore-not-found --wait >/dev/null 2>&1
sed 's#--kubeconfig=/etc/kubernetes/controller-manager.conf#--kubeconfig=/etc/kubernetes/controller-manger.conf#' /root/.kd/kube-controller-manager.yaml > /root/.kd/tmp.yaml
mv /root/.kd/tmp.yaml $f
for i in $(seq 1 30); do [ -z "$(crictl ps --name kube-controller-manager -q 2>/dev/null)" ] && break; sleep 2; done
kubectl create namespace team-stork --dry-run=client -o yaml | kubectl apply -f - >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ghost
  namespace: team-stork
  annotations:
    kubedojo.dev/original: "true"
spec:
  replicas: 2
  selector:
    matchLabels: {app: ghost}
  template:
    metadata:
      labels: {app: ghost}
    spec:
      containers:
        - name: pause
          image: registry.k8s.io/pause:3.10
YAML
