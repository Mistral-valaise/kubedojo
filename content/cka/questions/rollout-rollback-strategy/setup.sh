kubectl delete namespace team-dove --ignore-not-found --wait >/dev/null
kubectl create namespace team-dove >/dev/null
kubectl -n team-dove create deployment gateway --image=nginx:1.26-alpine --replicas=4 >/dev/null
kubectl -n team-dove rollout status deployment gateway --timeout=180s >/dev/null 2>&1
kubectl -n team-dove set image deployment gateway nginx=nginx:1.27.99-missing >/dev/null
sleep 3
