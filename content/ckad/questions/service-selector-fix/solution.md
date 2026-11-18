"Every second request fails" with healthy Pods means the Service balances over Pods that should not be
behind it. Look at who the Service selects:

```bash
ssh {{host}}
k -n app-skunk get svc orders -o wide                      # SELECTOR  app=orders
k -n app-skunk get pod -o wide --show-labels               # api-* AND worker-* carry app=orders
k -n app-skunk get endpointslices -l kubernetes.io/service-name=orders
#   four addresses: the two api Pods and the two worker Pods
```

The selector `app=orders` is too broad: both Deployments share that label. The worker Pods listen on
no port, so every connection that lands on them is refused. What tells the two apart is `tier`:

```bash
k -n app-skunk set selector svc orders 'app=orders,tier=api'
```

or `k -n app-skunk edit svc orders`:

```yaml
spec:
  selector:
    app: orders
    tier: api
```

Verify:

```bash
k -n app-skunk get endpointslices -l kubernetes.io/service-name=orders    # two addresses
k -n app-skunk get pod -l app=orders,tier=api -o wide                     # the same two IPs
for i in 1 2 3 4 5 6; do curl -s -o /dev/null -w '%{http_code}\n' -m 2 \
  "$(k -n app-skunk get svc orders -o jsonpath='{.spec.clusterIP}')"; done   # 200 every time
```

**Takeaway:** a Service selector is an AND over all its labels, and it selects every Pod in the
Namespace that matches, no matter which Deployment owns it. Fix the selector, not the Pods: a
Deployment's own selector is immutable, and other objects (here the workers' dashboards and policies)
may rely on the shared `app` label. `k get pod -l <selector>` shows exactly what a Service will pick.
