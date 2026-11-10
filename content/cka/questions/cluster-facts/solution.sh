set -e
cd /opt/course/facts
grep -o -- '--service-cluster-ip-range=[^ ]*' /etc/kubernetes/manifests/kube-apiserver.yaml | cut -d= -f2 > service-cidr
grep -o -- '--cluster-cidr=[^ ]*' /etc/kubernetes/manifests/kube-controller-manager.yaml | cut -d= -f2 > pod-cidr
grep -o -- '--authorization-mode=[^ ]*' /etc/kubernetes/manifests/kube-apiserver.yaml | cut -d= -f2 > authorization-modes
ls /etc/cni/net.d/ | head -1 > cni-config
