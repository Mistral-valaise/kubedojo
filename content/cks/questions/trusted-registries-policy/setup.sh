kubectl delete validatingadmissionpolicybinding trusted-registries --ignore-not-found >/dev/null
kubectl delete validatingadmissionpolicy trusted-registries --ignore-not-found >/dev/null
kubectl delete namespace sec-robin --ignore-not-found --wait >/dev/null
kubectl create namespace sec-robin >/dev/null
