kubectl delete namespace sec-finch --ignore-not-found --wait >/dev/null
kubectl create namespace sec-finch >/dev/null
mkdir -p /opt/course/tls
openssl req -x509 -newkey rsa:2048 -nodes -days 365 -subj "/CN=portal.finch.example" \
  -addext "subjectAltName=DNS:portal.finch.example" -keyout /opt/course/tls/portal.key -out /opt/course/tls/portal.crt >/dev/null 2>&1
chown -R candidate:candidate /opt/course/tls
kubectl -n sec-finch create service clusterip portal --tcp=80:80 >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: networking.k8s.io/v1
kind: IngressClass
metadata: {name: nginx}
spec: {controller: k8s.io/ingress-nginx}
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata: {name: portal, namespace: sec-finch}
spec:
  ingressClassName: nginx
  rules:
    - host: portal.finch.example
      http:
        paths:
          - {path: /, pathType: Prefix, backend: {service: {name: portal, port: {number: 80}}}}
YAML
