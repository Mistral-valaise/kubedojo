set -e
kubectl delete clusterrolebinding report-reader-binding
kubectl -n sec-heron create role report-reader --verb=get,list --resource=pods,configmaps
kubectl -n sec-heron create rolebinding report-reader --role=report-reader --serviceaccount=sec-heron:report-reader
