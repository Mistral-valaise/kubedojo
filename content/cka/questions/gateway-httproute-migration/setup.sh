kubectl get crd httproutes.gateway.networking.k8s.io >/dev/null 2>&1 || \
  kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.7.0/standard-install.yaml >/dev/null
kubectl wait --for=condition=Established crd/httproutes.gateway.networking.k8s.io crd/gateways.gateway.networking.k8s.io --timeout=90s >/dev/null
kubectl delete namespace team-plover --ignore-not-found --wait >/dev/null
kubectl create namespace team-plover >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: networking.k8s.io/v1
kind: IngressClass
metadata: {name: nginx}
spec: {controller: k8s.io/ingress-nginx}
---
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata: {name: dojo}
spec: {controllerName: kubedojo.dev/gateway-controller}
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata: {name: main, namespace: team-plover}
spec:
  gatewayClassName: dojo
  listeners:
    - {name: http, protocol: HTTP, port: 80}
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata: {name: portal, namespace: team-plover}
spec:
  ingressClassName: nginx
  rules:
    - host: portal.plover.example
      http:
        paths:
          - path: /docs
            pathType: Prefix
            backend: {service: {name: portal-docs, port: {number: 8080}}}
          - path: /
            pathType: Prefix
            backend: {service: {name: portal-web, port: {number: 80}}}
YAML
kubectl -n team-plover create service clusterip portal-web --tcp=80:80 >/dev/null
kubectl -n team-plover create service clusterip portal-docs --tcp=8080:80 >/dev/null
