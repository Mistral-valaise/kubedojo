Work from the outside in: **Service → endpoints → Pods → containers**.

```bash
ssh {{host}}
k -n team-lynx get deploy,pod,svc,endpointslices
```

**Problem 1 — Pods are in `ImagePullBackOff`.**

```bash
k -n team-lynx describe pod -l app=web | grep -A8 Events
# Failed to pull image "nginx:1.27-alpnie" ... not found
k -n team-lynx set image deployment/web nginx=nginx:1.27-alpine
k -n team-lynx rollout status deployment/web
```

**Problem 2 — the Service has no endpoints.**

```bash
k -n team-lynx get svc web -o wide          # SELECTOR app=web,tier=backend
k -n team-lynx get pod --show-labels        # app=web,tier=frontend
```

The selector does not match. Fix it with `k -n team-lynx edit svc web` and set `tier: frontend`.

**Problem 3 — endpoints exist, but connections are refused.**

The Service has `targetPort: 8080`, nginx listens on `80`. In the same edit, set `targetPort: 80`.

Verify from inside the cluster:

```bash
k -n team-lynx run tmp --rm -i --restart=Never --image=busybox:1.36 -- wget -qO- -T 3 web
```

**Takeaway:** `get endpointslices` (or `describe svc`) answers "does the Service find Pods?" in one command.
No endpoints → selector/readiness problem. Endpoints but no answer → port problem or NetworkPolicy.
