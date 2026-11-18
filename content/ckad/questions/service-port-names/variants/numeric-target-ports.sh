# expect: fail ports nodeport
# Everything is reachable, but targetPort hard-codes the container port numbers.
set -e
cat <<'YAML' | kubectl apply -f -
apiVersion: v1
kind: Service
metadata: {name: telemetry, namespace: app-weasel}
spec:
  selector: {app: telemetry}
  ports:
    - {name: web, port: 80, targetPort: 8080}
    - {name: metrics, port: 9090, targetPort: 9113}
---
apiVersion: v1
kind: Service
metadata: {name: telemetry-ext, namespace: app-weasel}
spec:
  type: NodePort
  selector: {app: telemetry}
  ports:
    - {name: web, port: 80, targetPort: 8080, nodePort: 31743}
YAML
