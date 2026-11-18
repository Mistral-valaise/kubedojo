Blue/green means: run the new version **next to** the old one, and move all traffic at once by
changing the Service selector. Nothing is ever half old, half new.

```bash
ssh {{host}}
k -n app-gecko get deploy,svc -o wide           # Service selector: app=api,color=blue
ip=$(k -n app-gecko get svc api -o jsonpath='{.spec.clusterIP}')
curl -sI $ip | grep -i server                   # Server: Apache/2.4.x
```

**1. Deploy green.** Generate the Deployment and fix the labels (the generator only sets `app=api-green`):

```bash
k -n app-gecko create deploy api-green --image=nginx:1.27-alpine --replicas=3 \
  --dry-run=client -o yaml > green.yaml
vim green.yaml
```

```yaml
spec:
  replicas: 3
  selector:
    matchLabels: {app: api, color: green}
  template:
    metadata:
      labels: {app: api, color: green}
```

```bash
k apply -f green.yaml
k -n app-gecko rollout status deploy/api-green        # wait: do not switch to Pods that are not ready
```

**2. Switch the Service** in place. A patch merges into the selector, so `app: api` stays:

```bash
k -n app-gecko patch svc api -p '{"spec":{"selector":{"color":"green"}}}'
# or: k -n app-gecko set selector svc api app=api,color=green
# or: k -n app-gecko edit svc api
```

**3. Verify.**

```bash
k -n app-gecko get endpointslices -l kubernetes.io/service-name=api     # the three green Pod IPs
curl -sI $ip | grep -i server                                           # Server: nginx/1.27.x
k -n app-gecko get deploy                                               # api-blue still 3/3
```

Blue keeps running, so the rollback is the same one-line patch with `color: blue`.

**Takeaway:** the Service is the switch. Deleting and recreating it would change its ClusterIP and drop
connections; editing the selector moves the traffic within a second and keeps the address.
