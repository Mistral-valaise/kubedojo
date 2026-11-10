kubectl delete namespace team-heron --ignore-not-found --wait >/dev/null
kubectl delete pv archive-pv --ignore-not-found --wait >/dev/null
kubectl create namespace team-heron >/dev/null
