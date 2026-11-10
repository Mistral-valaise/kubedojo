set -e
sed -i 's#controller-manger.conf#controller-manager.conf#' /etc/kubernetes/manifests/kube-controller-manager.yaml
