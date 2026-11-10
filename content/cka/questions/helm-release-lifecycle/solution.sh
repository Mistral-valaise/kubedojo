set -e
helm -n team-otter uninstall old-cache
helm -n team-otter install cache /opt/course/helm/webcache --set replicaCount=2 --set service.type=NodePort
kubectl -n team-otter rollout status deployment cache-webcache --timeout=120s
