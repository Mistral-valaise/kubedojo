kubectl delete clusterrolebinding kd-legacy-dashboard kd-node-inventory kd-inventory-reader --ignore-not-found >/dev/null
kubectl delete clusterrole kd-node-inventory --ignore-not-found >/dev/null
kubectl delete namespace sec-avocet-ops --ignore-not-found --wait >/dev/null
kubectl create namespace sec-avocet-ops >/dev/null
kubectl -n sec-avocet-ops create serviceaccount inventory >/dev/null
kubectl create clusterrole kd-node-inventory --verb=get,list,watch --resource=nodes >/dev/null
kubectl create clusterrolebinding kd-legacy-dashboard --clusterrole=view --group=system:unauthenticated >/dev/null
kubectl create clusterrolebinding kd-node-inventory --clusterrole=kd-node-inventory --user=system:anonymous >/dev/null
kubectl create clusterrolebinding kd-inventory-reader --clusterrole=view --serviceaccount=sec-avocet-ops:inventory >/dev/null
mkdir -p /opt/course/anonymous && rm -f /opt/course/anonymous/removed && chown -R candidate:candidate /opt/course/anonymous
