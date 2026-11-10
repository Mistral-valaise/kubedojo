kubectl delete namespace team-teal --ignore-not-found --wait >/dev/null
kubectl create namespace team-teal >/dev/null
kubectl -n team-teal create deployment renderer --image=nginx:1.27-alpine --replicas=1 >/dev/null
