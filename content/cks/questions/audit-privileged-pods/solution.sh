set -e
kubectl -n sec-wader get pod -o json | jq -r '.items[] | select(.spec.hostNetwork or .spec.hostPID or any(.spec.volumes[]?; .hostPath) or any(.spec.containers[]; .securityContext.privileged)) | .metadata.name' > /opt/course/audit/dangerous-pods
xargs kubectl -n sec-wader delete pod --wait=true < /opt/course/audit/dangerous-pods
