# expect: fail health-exact
# A trailing * on every rule makes /healthz a Prefix path: /healthz/anything would reach the API too.
set -e
kubectl -n app-walrus create ingress storefront --class=walrus-edge \
  --rule="store.walrus.example/healthz*=storefront-api:9090,tls=storefront-tls" \
  --rule="store.walrus.example/*=storefront:8080,tls=storefront-tls" \
  --default-backend=maintenance:80
