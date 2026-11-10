kubectl delete namespace sec-kite --ignore-not-found --wait >/dev/null
kubectl delete csr dana --ignore-not-found >/dev/null
kubectl create namespace sec-kite >/dev/null
kubectl -n sec-kite run inventory --image=registry.k8s.io/pause:3.10 >/dev/null
mkdir -p /opt/course/csr && rm -f /opt/course/csr/dana.crt
openssl genrsa -out /opt/course/csr/dana.key 2048 >/dev/null 2>&1
openssl req -new -key /opt/course/csr/dana.key -subj "/CN=dana/O=auditors" -out /opt/course/csr/dana.csr >/dev/null 2>&1
chown -R candidate:candidate /opt/course/csr
