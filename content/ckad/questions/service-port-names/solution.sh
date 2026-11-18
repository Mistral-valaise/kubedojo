set -e
cat <<'YAML' | kubectl apply -f -
apiVersion: v1
kind: Service
metadata: {name: telemetry, namespace: app-weasel}
spec:
  selector: {app: telemetry}
  ports:
    - {name: web, port: 80, targetPort: http}
    - {name: metrics, port: 9090, targetPort: metrics}
---
apiVersion: v1
kind: Service
metadata: {name: telemetry-ext, namespace: app-weasel}
spec:
  type: NodePort
  selector: {app: telemetry}
  ports:
    - {name: web, port: 80, targetPort: http, nodePort: 31743}
YAML
kubectl -n app-weasel rollout status deployment telemetry --timeout=120s
