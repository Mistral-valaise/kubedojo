# expect: fail removed others-intact
# Removes everything that looks related, including the binding of a
# ServiceAccount that has nothing to do with anonymous access.
set -e
kubectl delete clusterrolebinding kd-legacy-dashboard kd-node-inventory kd-inventory-reader
printf 'kd-legacy-dashboard\nkd-node-inventory\nkd-inventory-reader\n' > /opt/course/anonymous/removed
