set -e
cd /opt/course/kubeconfig
kubectl --kubeconfig fleet.yaml config get-contexts -o name > contexts
kubectl --kubeconfig fleet.yaml config current-context > current
kubectl --kubeconfig fleet.yaml config view --raw -o jsonpath='{.users[?(@.name=="lena")].user.client-certificate-data}' | base64 -d > lena.crt
