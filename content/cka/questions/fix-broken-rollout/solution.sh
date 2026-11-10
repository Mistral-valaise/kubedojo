set -e
kubectl -n team-lynx set image deployment/web nginx=nginx:1.27-alpine
kubectl -n team-lynx patch service web --type merge -p '{"spec":{"selector":{"app":"web","tier":"frontend"},"ports":[{"port":80,"targetPort":80}]}}'
kubectl -n team-lynx rollout status deployment/web --timeout=120s
