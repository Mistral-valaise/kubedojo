set -e
kubectl -n team-dove rollout undo deployment gateway
kubectl -n team-dove rollout status deployment gateway --timeout=120s
kubectl -n team-dove patch deployment gateway -p '{"spec":{"strategy":{"type":"RollingUpdate","rollingUpdate":{"maxUnavailable":0,"maxSurge":1}}}}'
kubectl -n team-dove set image deployment gateway nginx=nginx:1.27-alpine
kubectl -n team-dove rollout status deployment gateway --timeout=180s
