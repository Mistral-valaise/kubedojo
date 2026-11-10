set -e
kubectl -n team-raven patch deployment api --type json -p '[
 {"op":"replace","path":"/spec/template/spec/containers/0/env/0/valueFrom/secretKeyRef/key","value":"token"},
 {"op":"replace","path":"/spec/template/spec/containers/0/livenessProbe/httpGet/port","value":80}]'
kubectl -n team-raven rollout status deployment api --timeout=120s
