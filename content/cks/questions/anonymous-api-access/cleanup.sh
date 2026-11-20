kubectl delete clusterrolebinding kd-legacy-dashboard kd-node-inventory kd-inventory-reader --ignore-not-found >/dev/null
kubectl delete clusterrole kd-node-inventory --ignore-not-found >/dev/null
exit 0
