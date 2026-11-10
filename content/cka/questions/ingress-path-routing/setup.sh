kubectl delete namespace team-wren --ignore-not-found --wait >/dev/null
kubectl create namespace team-wren >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: networking.k8s.io/v1
kind: IngressClass
metadata: {name: nginx}
spec: {controller: k8s.io/ingress-nginx}
YAML
kubectl -n team-wren create deployment shop --image=nginx:1.27-alpine --port=80 >/dev/null
kubectl -n team-wren create deployment shop-api --image=nginx:1.27-alpine --port=80 >/dev/null
kubectl -n team-wren expose deployment shop --port=80 >/dev/null
kubectl -n team-wren expose deployment shop-api --port=8080 --target-port=80 >/dev/null
