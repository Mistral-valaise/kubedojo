```bash
ssh {{host}}
k -n team-quail-dev create service externalname billing \
  --external-name billing.team-quail-prod.svc.cluster.local
echo billing.team-quail-prod.svc.cluster.local > /opt/course/quail/fqdn
```

```yaml
apiVersion: v1
kind: Service
metadata:
  name: billing
  namespace: team-quail-dev
spec:
  type: ExternalName
  externalName: billing.team-quail-prod.svc.cluster.local
```

Verify from a Pod in the dev Namespace:

```bash
k -n team-quail-dev run tmp --rm -i --restart=Never --image=busybox:1.36 -- nslookup billing
# billing.team-quail-dev.svc.cluster.local  canonical name = billing.team-quail-prod.svc.cluster.local
```

**Why the short name works:** a Pod's `/etc/resolv.conf` has the search list
`<its-namespace>.svc.cluster.local svc.cluster.local cluster.local`. `billing` is tried as
`billing.team-quail-dev.svc.cluster.local` first — exactly the alias you created.
