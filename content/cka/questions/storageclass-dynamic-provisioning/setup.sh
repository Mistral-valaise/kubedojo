kubectl delete namespace team-pelican --ignore-not-found --wait >/dev/null
for pv in $(kubectl get pv -o jsonpath='{range .items[?(@.spec.storageClassName=="archive-retain")]}{.metadata.name}{"\n"}{end}'); do
  kubectl delete pv "$pv" --wait=false >/dev/null
done
kubectl delete storageclass archive-retain --ignore-not-found >/dev/null
kubectl create namespace team-pelican >/dev/null
