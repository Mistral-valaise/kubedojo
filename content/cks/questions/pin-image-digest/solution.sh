set -e
id=$(kubectl -n sec-swift get pod -l app=edge -o jsonpath='{.items[0].status.containerStatuses[0].imageID}')
digest=$(echo "$id" | grep -o 'sha256:[a-f0-9]*')
echo "$digest" > /opt/course/digest/edge
kubectl -n sec-swift set image deployment edge nginx=docker.io/library/nginx@$digest
kubectl -n sec-swift rollout status deployment edge --timeout=180s
