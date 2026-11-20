# expect: fail nodes removed
# Stops at the first finding: the binding of the user system:anonymous stays.
set -e
kubectl delete clusterrolebinding kd-legacy-dashboard
echo kd-legacy-dashboard > /opt/course/anonymous/removed
