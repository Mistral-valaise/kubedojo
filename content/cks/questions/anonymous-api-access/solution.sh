set -e
kubectl delete clusterrolebinding kd-legacy-dashboard kd-node-inventory
printf 'kd-legacy-dashboard\nkd-node-inventory\n' > /opt/course/anonymous/removed
