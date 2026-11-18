kubectl delete namespace app-koala --ignore-not-found --wait >/dev/null
kubectl create namespace app-koala >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata:
  name: shop
  namespace: app-koala
spec:
  replicas: 3
  selector:
    matchLabels: {app: shop}
  template:
    metadata:
      labels: {app: shop}
    spec:
      terminationGracePeriodSeconds: 2
      containers:
        - name: web
          image: busybox:1.36
          command:
            - sh
            - -c
            - |
              mkdir -p /www
              echo "shop: warming up the catalog cache"
              (sleep 12; echo '<h1>shop</h1>' > /www/index.html; echo ready > /www/ready; echo "shop: ready") &
              exec httpd -f -p 8080 -h /www
          ports:
            - name: http
              containerPort: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: shop
  namespace: app-koala
spec:
  selector: {app: shop}
  ports:
    - port: 80
      targetPort: http
YAML
kubectl -n app-koala rollout status deploy/shop --timeout=180s >/dev/null 2>&1 || true
