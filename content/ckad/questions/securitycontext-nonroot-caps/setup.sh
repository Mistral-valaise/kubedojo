kubectl delete namespace app-puma --ignore-not-found --wait >/dev/null
kubectl create namespace app-puma >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: v1
kind: ConfigMap
metadata: {name: edge-page, namespace: app-puma}
data:
  index.html: |
    puma edge ok
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: edge, namespace: app-puma}
spec:
  replicas: 1
  selector: {matchLabels: {app: edge}}
  template:
    metadata: {labels: {app: edge}}
    spec:
      terminationGracePeriodSeconds: 1
      volumes:
        - name: page
          configMap: {name: edge-page}
      containers:
        - name: httpd
          image: busybox:1.36
          command: ["httpd", "-f", "-v", "-p", "80", "-h", "/www"]
          ports: [{containerPort: 80}]
          readinessProbe:
            httpGet: {path: /, port: 80}
            periodSeconds: 2
          securityContext:
            allowPrivilegeEscalation: true
            capabilities:
              add: [NET_ADMIN, NET_BIND_SERVICE]
          volumeMounts:
            - {name: page, mountPath: /www}
YAML
kubectl -n app-puma rollout status deployment edge --timeout=180s >/dev/null 2>&1 || true
