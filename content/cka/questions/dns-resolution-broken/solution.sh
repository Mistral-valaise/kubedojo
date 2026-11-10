set -e
kubectl -n kube-system get cm coredns -o yaml | sed 's/kubernetes cluster\.lokal/kubernetes cluster.local/' | kubectl apply -f -
