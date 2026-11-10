kubectl delete namespace sec-pelican --ignore-not-found --wait >/dev/null
kubectl create namespace sec-pelican >/dev/null
kubectl -n sec-pelican create secret generic db-pass --from-literal=password=pelican-4711 >/dev/null
mkdir -p /root/.kd
