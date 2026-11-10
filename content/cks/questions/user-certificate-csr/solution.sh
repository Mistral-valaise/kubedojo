set -e
cat <<YAML | kubectl apply -f -
apiVersion: certificates.k8s.io/v1
kind: CertificateSigningRequest
metadata: {name: dana}
spec:
  request: $(base64 -w0 /opt/course/csr/dana.csr)
  signerName: kubernetes.io/kube-apiserver-client
  usages: [client auth]
YAML
kubectl certificate approve dana
for i in $(seq 1 20); do
  c=$(kubectl get csr dana -o jsonpath='{.status.certificate}'); [ -n "$c" ] && break; sleep 1
done
echo "$c" | base64 -d > /opt/course/csr/dana.crt
kubectl -n sec-kite create role pod-auditor --verb=get,list --resource=pods
kubectl -n sec-kite create rolebinding pod-auditor --role=pod-auditor --user=dana
