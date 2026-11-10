f=/etc/kubernetes/manifests/kube-scheduler.yaml
mkdir -p /root/.kd; [ -f /root/.kd/kube-scheduler.yaml ] || cp $f /root/.kd/kube-scheduler.yaml
kubectl delete pod reports -n team-ibis --ignore-not-found --wait=false --grace-period=0 --force >/dev/null 2>&1
sed 's#--kubeconfig=/etc/kubernetes/scheduler.conf#--kubeconfig=/etc/kubernetes/scheduler.config#' /root/.kd/kube-scheduler.yaml > /root/.kd/tmp.yaml
mv /root/.kd/tmp.yaml $f
for i in $(seq 1 30); do [ -z "$(crictl ps --name kube-scheduler -q 2>/dev/null)" ] && break; sleep 2; done
kubectl create namespace team-ibis --dry-run=client -o yaml | kubectl apply -f - >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: v1
kind: Pod
metadata:
  name: reports
  namespace: team-ibis
  annotations:
    kubedojo.dev/original: "true"
spec:
  nodeSelector:
    node-role.kubernetes.io/control-plane: ""
  tolerations:
    - key: node-role.kubernetes.io/control-plane
      effect: NoSchedule
  containers:
    - name: reports
      image: nginx:1.27-alpine
YAML
