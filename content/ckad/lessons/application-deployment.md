# Application Deployment

**20% of the exam.** Getting a new version out, and back: rolling updates, blue/green, canary, Helm and Kustomize.

## The 20% that solves 80%

### Rolling updates

```bash
k set image deploy/web nginx=nginx:1.27-alpine      # container=image; starts a rollout
k rollout status deploy/web                         # blocks until done (or shows where it hangs)
k rollout history deploy/web [--revision=3]         # revisions; with --revision the Pod template
k rollout undo deploy/web [--to-revision=2]         # back to the previous (or a given) revision
k rollout restart deploy/web                        # same template, new Pods
```

Only a change to `spec.template` creates a revision. Scaling does not.

```yaml
spec:
  strategy:
    type: RollingUpdate            # or Recreate: all old Pods stop first (downtime)
    rollingUpdate:
      maxSurge: 1                  # extra Pods above replicas during the update
      maxUnavailable: 0            # Pods that may be missing; 0 = never below replicas
```

Both take a number or a percentage, the default is `25%` each. `maxSurge: 0` with `maxUnavailable: 1`
updates without extra capacity; both `0` is rejected. A rollout only proceeds when new Pods become
**Ready**, so a readinessProbe is what makes it safe.

### Blue/green and canary: the Service selector is the switch

```text
blue/green                              canary
Service  selector: app=api,color=blue   Service  selector: app=shop
   └─▶ api-blue   (3 Pods)                 ├─▶ shop-stable  (4 Pods)   80%
       api-green  (3 Pods, idle)           └─▶ shop-canary  (1 Pod)    20%
```

- **Blue/green:** second Deployment with a label that differs (`color=green`), wait for `rollout status`,
  then change the selector. Switching back is the same command. Never recreate the Service: its IP changes.
- **Canary:** second Deployment whose Pods carry the label the Service selects **plus** one of their own
  (`track=canary`). Traffic share = share of Pods. Keep the total: scale stable down by what the canary adds.

```bash
k get deploy web -o yaml > green.yaml               # copy; change name, selector, labels, image
k patch svc api -p '{"spec":{"selector":{"color":"green"}}}'
k set selector svc api app=api,color=green          # replaces the whole selector
k get endpointslices -l kubernetes.io/service-name=api      # which Pods get traffic right now
```

### Helm

```bash
helm -n team ls [-a]                                # releases (-a: also failed and pending)
helm show values ./chart                            # what a chart lets you set
helm -n team install web ./chart --set replicaCount=2 -f my-values.yaml
helm -n team upgrade web ./chart --reuse-values --set image.tag=1.27-alpine
helm -n team history web                            # revisions
helm -n team get values web [--revision 2] [-a]     # values of a revision (-a: with defaults)
helm -n team rollback web 2                         # creates a NEW revision with the content of 2
helm repo add NAME URL && helm repo update && helm search repo NAME
```

- Releases live in a Namespace: every command needs `-n`.
- `upgrade` without `--reuse-values` starts from the chart defaults; earlier `--set` values are gone.
- Status `deployed` does not mean the Pods run. Helm only waits with `--wait`; look at `k get pod`.

### Kustomize

```yaml
# overlays/prod/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources: [../../base]
namespace: prod
namePrefix: prod-
labels: [{pairs: {env: prod}}]
images: [{name: nginx, newTag: 1.27-alpine}]       # name as in the base, without tag
replicas: [{name: web, count: 3}]                  # name as in the base, without prefix
configMapGenerator:
  - name: web-config                         # rendered as prod-web-config-<hash>
    literals: [MODE=prod]
```

```bash
k kustomize overlays/prod && k apply -k overlays/prod      # render to look, then apply
```

Generated ConfigMaps and Secrets get a content hash in their name and every reference is rewritten,
so a config change rolls the Pods. What has no built-in field goes into `patches:`.

## Habits that save points

- After every change to a Deployment: `k rollout status`. A task is not solved while Pods are still pulling.
- A stuck rollout keeps the old Pods serving. `k get pod` and `k describe pod` show why the new ones
  fail; `k rollout undo` or `helm rollback` is usually faster than repairing forward.
- Check the Service, not the Deployment: `k get endpointslices` or `curl <ClusterIP>` from the node.
- Read which revision or which share is asked for: "the previous revision" and "revision 1" are
  often not the same.
