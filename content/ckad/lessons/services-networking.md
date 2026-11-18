# Services and Networking

**20% of the exam.** Services, Ingress rules and NetworkPolicies, seen from the application: you rarely
build the network, you make your workload fit into it. Almost every task comes down to **labels**.

## The 20% that solves 80%

### Services select Pods by label

```bash
k expose deploy web --name web --port 80 --target-port 8080           # ClusterIP, selector copied
k set selector svc web 'app=web,tier=api'                             # replace the selector
k get svc web -o wide                                                 # SELECTOR column
k get pod -l app=web,tier=api -o wide                                 # what that selector picks
k get endpointslices -l kubernetes.io/service-name=web                # what the Service really uses
```

- A selector is an **AND** over its labels and matches every Pod in the Namespace, whichever
  Deployment owns it. Too broad: foreign Pods get traffic (intermittent errors). Too narrow or
  misspelled: no endpoints (connection refused).
- Only **ready** Pods become endpoints. Failing readiness probe = Pod running, but not behind the Service.

### port, targetPort, nodePort

```yaml
spec:
  type: NodePort                 # omit for ClusterIP
  selector: {app: web}
  ports:
    - name: web                  # mandatory when there is more than one port
      port: 80                   # clients inside the cluster: web:80
      targetPort: http           # container port, by number or by NAME
      nodePort: 31080            # <any node IP>:31080, range 30000-32767
    - name: metrics
      port: 9090
      targetPort: metrics
```

A named `targetPort` is looked up in `containers[].ports[].name` of each Pod, so the container port
can change without touching the Service. `k expose` produces neither port names, named target ports
nor a fixed `nodePort`: generate with `$do`, edit, apply, then check `k get endpointslices`.

### Ingress

```bash
k get ingressclass
k create ingress shop --class=nginx \
  --rule="shop.example.com/healthz=api:9090,tls=shop-tls" \
  --rule="shop.example.com/*=web:8080,tls=shop-tls" \
  --default-backend=fallback:80 $do
```

`host/path=svc:port` gives `pathType: Exact`, a trailing `*` gives `Prefix` (`/*` = everything);
`,tls=secret` adds the `spec.tls` entry; `--default-backend` is used when no rule matches.

The backend port is the **Service** port, not the container port. The TLS Secret is of type
`kubernetes.io/tls` and lives in the Ingress's Namespace:
`k create secret tls shop-tls --cert=tls.crt --key=tls.key`. Read the result with
`k describe ingress shop`: it resolves the backends and flags Services that do not exist.

### NetworkPolicy: read it, then match it

1. No policy selects a Pod: everything is allowed.
2. A policy selects a Pod for a direction (`policyTypes`): that direction is denied except what is listed.
3. Policies only add allowances. There is no deny rule and no order.

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: db-ingress, namespace: shop}
spec:
  podSelector: {matchLabels: {app: db}}                 # who this policy applies to
  policyTypes: [Ingress]
  ingress:
    - from:
        - podSelector: {matchLabels: {db-access: "true"}}   # who may connect
      ports: [{protocol: TCP, port: 5432}]              # port of the destination Pod
```

The typical CKAD task gives you finished policies and forbids changing them: **make the Pod carry
the labels the policies select on.**

```bash
k describe netpol                          # per policy: PodSelector, and From / To peers
k get pod --show-labels
k label pod web-1 db-access=true           # a bare Pod
k edit deploy web                          # Deployment: spec.template.metadata.labels
```

With a default deny for both directions a connection needs **two** allowances: egress for the client
and ingress for the server. They may hang on two different labels. Egress denied without an allowance
for port 53 also breaks DNS: test with the Service name and with the IP to tell them apart.

## Habits that save points

- Timeout = NetworkPolicy. Connection refused = no endpoint, wrong targetPort or nothing listening.
- `k label deploy web x=y` labels the Deployment object, not its Pods. Policies and Services see only
  Pod labels; for a Deployment they come from the template, and changing them triggers a rollout.
- Test like the client does: `k exec deploy/web -- wget -qO- -T 3 http://db:80`, and test that what
  must stay blocked still is. `wget -T` / `nc -w` keep a blocked probe from hanging.
