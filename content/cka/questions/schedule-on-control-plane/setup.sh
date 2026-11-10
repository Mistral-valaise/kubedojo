kubectl delete namespace team-wader --ignore-not-found --wait >/dev/null
kubectl create namespace team-wader >/dev/null
kubectl taint node ${KD_HOST}-control-plane node-role.kubernetes.io/control-plane=:NoSchedule --overwrite >/dev/null 2>&1 || true
