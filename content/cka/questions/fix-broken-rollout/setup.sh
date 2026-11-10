kubectl delete namespace team-lynx --ignore-not-found --wait >/dev/null
kubectl create namespace team-lynx >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: team-lynx
  annotations:
    kubedojo.dev/original: "true"
spec:
  replicas: 3
  selector:
    matchLabels: {app: web, tier: frontend}
  template:
    metadata:
      labels: {app: web, tier: frontend}
    spec:
      containers:
        - name: nginx
          image: nginx:1.27-alpnie
          ports:
            - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: web
  namespace: team-lynx
spec:
  selector: {app: web, tier: backend}
  ports:
    - port: 80
      targetPort: 8080
YAML
