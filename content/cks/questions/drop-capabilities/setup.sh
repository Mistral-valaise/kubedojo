kubectl delete namespace sec-egret --ignore-not-found --wait >/dev/null
kubectl create namespace sec-egret >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata: {name: gateway, namespace: sec-egret}
spec:
  replicas: 1
  selector: {matchLabels: {app: gateway}}
  template:
    metadata: {labels: {app: gateway}}
    spec:
      terminationGracePeriodSeconds: 1
      containers:
        - name: gateway
          image: busybox:1.36
          command: ["sh", "-c", "mkdir -p /www && echo gateway-ok > /www/index.html && chown -R 65534:65534 /www && exec httpd -f -p 80 -h /www"]
          ports: [{containerPort: 80}]
          securityContext:
            capabilities:
              add: ["NET_ADMIN", "SYS_TIME"]
YAML
kubectl -n sec-egret rollout status deployment gateway --timeout=120s >/dev/null 2>&1 || true
