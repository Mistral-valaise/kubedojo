```bash
ssh {{host}}
cd /opt/course/csr
openssl req -in dana.csr -noout -subject        # subject=CN = dana, O = auditors
base64 -w0 dana.csr; echo
```

```yaml
apiVersion: certificates.k8s.io/v1
kind: CertificateSigningRequest
metadata:
  name: dana
spec:
  request: LS0tLS1CRUdJTi...        # the base64 output, one line
  signerName: kubernetes.io/kube-apiserver-client
  usages:
    - client auth
```

```bash
k apply -f csr.yaml
k get csr                                         # dana   Pending
k certificate approve dana
k get csr dana -o jsonpath='{.status.certificate}' | base64 -d > dana.crt
openssl x509 -in dana.crt -noout -subject -issuer
```

Authorization is separate — the certificate only proves *who* she is:

```bash
k -n sec-kite create role pod-auditor --verb=get,list --resource=pods
k -n sec-kite create rolebinding pod-auditor --role=pod-auditor --user=dana
k -n sec-kite auth can-i list pods --as dana      # yes
```

To hand it over you would build a kubeconfig:

```bash
k config set-credentials dana --client-certificate=dana.crt --client-key=dana.key --embed-certs
k config set-context dana --cluster=kubernetes --user=dana --namespace=sec-kite
```

Certificates cannot be revoked in Kubernetes: keep lifetimes short (`spec.expirationSeconds`) and remove
the RoleBinding when access is no longer needed.
