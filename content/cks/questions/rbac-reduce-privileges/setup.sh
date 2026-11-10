kubectl delete namespace sec-heron --ignore-not-found --wait >/dev/null
kubectl delete clusterrolebinding report-reader-binding --ignore-not-found >/dev/null
kubectl create namespace sec-heron >/dev/null
kubectl -n sec-heron create serviceaccount report-reader >/dev/null
kubectl create clusterrolebinding report-reader-binding --clusterrole=cluster-admin --serviceaccount=sec-heron:report-reader >/dev/null
kubectl -n sec-heron create configmap report-settings --from-literal=interval=5m >/dev/null
