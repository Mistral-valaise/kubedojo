set -e
cat <<'YAML' | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: shop-canary
  namespace: app-ferret
spec:
  replicas: 1
  selector:
    matchLabels: {app: shop, track: canary}
  template:
    metadata:
      labels: {app: shop, track: canary}
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
              value: v2
          ports:
            - containerPort: 80
          readinessProbe:
            httpGet: {path: /, port: 80}
            periodSeconds: 2
YAML
kubectl -n app-ferret rollout status deployment/shop-canary --timeout=120s
kubectl -n app-ferret scale deployment/shop-stable --replicas=4
kubectl -n app-ferret rollout status deployment/shop-stable --timeout=120s
