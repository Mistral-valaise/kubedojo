kubectl delete namespace app-walrus --ignore-not-found --wait >/dev/null
kubectl create namespace app-walrus >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: networking.k8s.io/v1
kind: IngressClass
metadata: {name: walrus-edge}
spec: {controller: kubedojo.dev/walrus-edge}
YAML
mk() { cat <<YAML | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata: {name: $1, namespace: app-walrus}
spec:
  replicas: 1
  selector: {matchLabels: {app: $1}}
  template:
    metadata: {labels: {app: $1}}
    spec:
      containers:
        - {name: nginx, image: "nginx:1.27-alpine", ports: [{name: http, containerPort: 80}]}
---
apiVersion: v1
kind: Service
metadata: {name: $1, namespace: app-walrus}
spec:
  selector: {app: $1}
  ports: [{name: http, port: $2, targetPort: http}]
YAML
}
mk storefront 8080; mk storefront-api 9090; mk maintenance 80
tmp=$(mktemp -d)
openssl req -x509 -newkey rsa:2048 -nodes -days 30 -subj "/CN=store.walrus.example" \
  -addext "subjectAltName=DNS:store.walrus.example" \
  -keyout "$tmp/tls.key" -out "$tmp/tls.crt" >/dev/null 2>&1
kubectl -n app-walrus create secret tls storefront-tls --cert="$tmp/tls.crt" --key="$tmp/tls.key" >/dev/null
rm -rf "$tmp"
