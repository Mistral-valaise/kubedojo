set -e
sed -i 's#--kubeconfig=/etc/kubernetes/scheduler.config#--kubeconfig=/etc/kubernetes/scheduler.conf#' /etc/kubernetes/manifests/kube-scheduler.yaml
