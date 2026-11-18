kubectl delete namespace app-ocelot --ignore-not-found --wait >/dev/null
kubectl create namespace app-ocelot >/dev/null
mkdir -p /opt/course/ocelot && rm -rf /opt/course/ocelot/*
cat > /opt/course/ocelot/gateway.conf <<'CONF'
server {
    listen 8080;
    location /healthz {
        default_type text/plain;
        return 200 "ocelot gateway ok\n";
    }
    location / {
        return 404;
    }
}
CONF
chown -R candidate:candidate /opt/course/ocelot
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata: {name: gateway, namespace: app-ocelot}
spec:
  replicas: 1
  selector: {matchLabels: {app: gateway}}
  template:
    metadata: {labels: {app: gateway}}
    spec:
      terminationGracePeriodSeconds: 1
      containers:
        - name: nginx
          image: nginx:1.27-alpine
YAML
kubectl -n app-ocelot rollout status deployment gateway --timeout=180s >/dev/null 2>&1 || true
