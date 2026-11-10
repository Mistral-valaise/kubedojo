```bash
ssh {{host}}
sudo openssl x509 -in /etc/kubernetes/pki/apiserver.crt -noout -enddate
# notAfter=Nov 10 21:31:09 2027 GMT
```

Write the date (without `notAfter=`) to the file:

```bash
sudo openssl x509 -in /etc/kubernetes/pki/apiserver.crt -noout -enddate | cut -d= -f2 > /opt/course/certs/expiration
```

kubeadm shows the same information for all certificates:

```bash
sudo kubeadm certs check-expiration
# CERTIFICATE                EXPIRES                  RESIDUAL TIME   CERTIFICATE AUTHORITY
# apiserver                  Nov 10, 2027 21:31 UTC   364d            ca
# apiserver-etcd-client      ...
```

The name in the first column is the argument for `renew`:

```bash
echo "kubeadm certs renew apiserver" > /opt/course/certs/renew.sh
```

After a real renewal the static Pods must be restarted so they load the new certificate.
