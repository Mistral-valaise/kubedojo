```bash
ssh {{host}}
k -n sec-ibis exec deploy/static-site -- ls /var/run/secrets/kubernetes.io/serviceaccount
# ca.crt  namespace  token                <- the credential we want gone
```

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: static-site
  namespace: sec-ibis
automountServiceAccountToken: false
```

```bash
k apply -f sa.yaml
k -n sec-ibis set serviceaccount deploy static-site static-site
k -n sec-ibis rollout status deploy static-site
k -n sec-ibis exec deploy/static-site -- ls /var/run/secrets/kubernetes.io/serviceaccount
# ls: /var/run/secrets/kubernetes.io/serviceaccount: No such file or directory
```

**Precedence:** `automountServiceAccountToken` in the **Pod spec** wins over the ServiceAccount.
So for a single workload you can also set it there:

```yaml
spec:
  template:
    spec:
      automountServiceAccountToken: false
```

Workloads that do need the API keep the token — and get a Role with exactly the permissions they use.
