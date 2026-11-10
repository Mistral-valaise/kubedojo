set -e
kubectl -n team-quail-dev create service externalname billing --external-name billing.team-quail-prod.svc.cluster.local
echo billing.team-quail-prod.svc.cluster.local > /opt/course/quail/fqdn
