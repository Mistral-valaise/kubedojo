kubectl delete namespace app-ferret --ignore-not-found --wait >/dev/null
kubectl create namespace app-ferret >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata:
  name: shop-stable
  namespace: app-ferret
spec:
  replicas: 5
  selector:
    matchLabels: {app: shop, track: stable}
  template:
    metadata:
      labels: {app: shop, track: stable}
    spec:
      terminationGracePeriodSeconds: 2
      containers:
        - name: shop
          image: nginx:1.27-alpine
          command: ["/bin/sh", "-c"]
          args:
            - echo "shop $APP_VERSION" > /usr/share/nginx/html/index.html && exec nginx -g "daemon off;"
          env:
            - name: APP_VERSION
              value: v1
          ports:
            - containerPort: 80
          readinessProbe:
            httpGet: {path: /, port: 80}
            periodSeconds: 2
---
apiVersion: v1
kind: Service
metadata:
  name: shop
  namespace: app-ferret
spec:
  selector: {app: shop}
  ports:
    - port: 80
      targetPort: 80
YAML
kubectl -n app-ferret rollout status deploy/shop-stable --timeout=180s >/dev/null 2>&1 || true
