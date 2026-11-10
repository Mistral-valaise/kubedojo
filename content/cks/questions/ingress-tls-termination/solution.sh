set -e
kubectl -n sec-finch create secret tls portal-tls --cert=/opt/course/tls/portal.crt --key=/opt/course/tls/portal.key
kubectl -n sec-finch patch ingress portal --type merge -p '{"spec":{"tls":[{"hosts":["portal.finch.example"],"secretName":"portal-tls"}]}}'
