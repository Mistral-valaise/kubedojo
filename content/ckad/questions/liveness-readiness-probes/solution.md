Probes belong to the **container**, and each probe is one handler plus timings.

```bash
ssh {{host}}
k -n app-koala get deploy,pod,svc
k -n app-koala edit deploy shop
```

```yaml
      containers:
        - name: web
          image: busybox:1.36
          readinessProbe:
            httpGet:
              path: /ready
              port: 8080            # or the port's name: http
            periodSeconds: 5
          livenessProbe:
            tcpSocket:
              port: 8080
            initialDelaySeconds: 10
            periodSeconds: 10
```

Saving the edit starts a rollout. Watch it:

```bash
k -n app-koala rollout status deploy shop
k -n app-koala get pod            # new Pods stay 0/1 for ~12s, then 1/1
k -n app-koala describe pod -l app=shop | grep -E 'Liveness|Readiness'
#   Liveness:   tcp-socket :8080 delay=10s timeout=1s period=10s #success=1 #failure=3
#   Readiness:  http-get http://:8080/ready delay=0s timeout=1s period=5s #success=1 #failure=3
```

While a Pod warms up you see the readiness probe at work in its Events
(`Readiness probe failed: HTTP probe failed with statuscode: 404`) and the Pod is missing from the
Service:

```bash
k -n app-koala get endpointslices -l kubernetes.io/service-name=shop
```

**Takeaway:** readiness decides *traffic* (endpoints, rollout progress), liveness decides
*restarts*. Timings (`initialDelaySeconds`, `periodSeconds`, `failureThreshold`) are siblings of
`httpGet`/`tcpSocket`/`exec`, not children. `k explain pod.spec.containers.livenessProbe` lists
them when you forget a name.
