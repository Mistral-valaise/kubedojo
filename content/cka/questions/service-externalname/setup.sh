kubectl delete namespace team-quail-dev team-quail-prod --ignore-not-found --wait >/dev/null
kubectl create namespace team-quail-dev >/dev/null
kubectl create namespace team-quail-prod >/dev/null
mkdir -p /opt/course/quail && rm -f /opt/course/quail/fqdn && chown -R candidate:candidate /opt/course/quail
kubectl -n team-quail-prod create deployment billing --image=nginx:1.27-alpine --port=80 >/dev/null
kubectl -n team-quail-prod expose deployment billing --port=80 >/dev/null
