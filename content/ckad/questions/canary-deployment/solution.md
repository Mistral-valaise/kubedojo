A canary with plain Kubernetes is nothing but **two Deployments behind one Service**. The Service
balances over all ready Pods its selector matches, so the share of traffic is the share of Pods.

```bash
ssh {{host}}
k -n app-ferret get deploy,svc -o wide          # Service selector: app=shop
k -n app-ferret get pod --show-labels           # app=shop,track=stable
```

The Service selects only `app=shop`. A second Deployment whose Pods carry `app=shop` too is picked up
automatically. It needs its **own** selector, though, or the two Deployments would claim each other's
Pods: keep `track` and give it another value.

Copy the stable Deployment instead of typing a new one:

```bash
k -n app-ferret get deploy shop-stable -o yaml > canary.yaml
vim canary.yaml
```

Remove `status`, `resourceVersion`, `uid`, `creationTimestamp` and the annotations, then change four things:

```yaml
metadata:
  name: shop-canary                 # 1
  namespace: app-ferret
spec:
  replicas: 1                       # 2: 1 of 5 Pods = 20%
  selector:
    matchLabels: {app: shop, track: canary}      # 3
  template:
    metadata:
      labels: {app: shop, track: canary}         # 3
    spec:
      containers:
        - name: shop
          env:
            - name: APP_VERSION
              value: v2             # 4
          # image, command, args, ports, readinessProbe unchanged
```

```bash
k apply -f canary.yaml
k -n app-ferret rollout status deploy/shop-canary
k -n app-ferret scale deploy/shop-stable --replicas=4      # total stays at 5
```

Start the canary first and shrink stable afterwards, so capacity never drops below 5 Pods.

Verify what the Service really sends traffic to:

```bash
k -n app-ferret get endpointslices -l kubernetes.io/service-name=shop      # 5 addresses
ip=$(k -n app-ferret get svc shop -o jsonpath='{.spec.clusterIP}')
for i in $(seq 20); do curl -s $ip; done | sort | uniq -c
#     16 shop v1
#      4 shop v2        (roughly; the split is random per connection)
```

**Takeaway:** the canary share is `canary replicas / all replicas behind the Service`. Promote by
scaling canary up and stable down, abort by scaling the canary to 0. Finer splits than the replica
ratio need an ingress controller or a service mesh.
