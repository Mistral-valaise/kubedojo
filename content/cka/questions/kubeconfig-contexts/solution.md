```bash
ssh {{host}}
cd /opt/course/kubeconfig
export KUBECONFIG=$PWD/fleet.yaml        # or pass --kubeconfig fleet.yaml each time

k config get-contexts -o name > contexts
k config current-context > current
```

For the certificate, `--raw` is essential — without it the data is replaced by `DATA+OMITTED`:

```bash
k config view --raw -o jsonpath='{.users[?(@.name=="lena")].user.client-certificate-data}' | base64 -d > lena.crt
openssl x509 -in lena.crt -noout -subject        # subject=CN = lena, O = fleet
unset KUBECONFIG
```

Reading the file in an editor and copying the value by hand is just as valid. Note that `lena` is the
**third** entry in `users` — selecting by index (`users[1]`) would return the wrong certificate.

Do not forget `unset KUBECONFIG`, or the following tasks run against the wrong cluster.
