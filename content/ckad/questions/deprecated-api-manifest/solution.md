Let the cluster tell you what is wrong, one message at a time.

```bash
ssh {{host}}
cd /opt/course/marmot
k apply -f .
# no matches for kind "Deployment" in version "extensions/v1beta1"
# no matches for kind "CronJob" in version "batch/v1beta1"
# no matches for kind "HorizontalPodAutoscaler" in version "autoscaling/v2beta1"
k api-resources | grep -i -E 'deployments|cronjobs|horizontalpod|disruption'
# deployments                deploy   apps/v1          true   Deployment
# cronjobs                   cj       batch/v1         true   CronJob
# horizontalpodautoscalers   hpa      autoscaling/v2   true   HorizontalPodAutoscaler
# poddisruptionbudgets       pdb      policy/v1        true   PodDisruptionBudget
```

**cronjob.yaml** — only the version changes: `apiVersion: batch/v1`.

**deployment.yaml** — `apiVersion: apps/v1`, then validate against the server:

```bash
k apply --dry-run=server -f deployment.yaml
# ... spec.selector: Required value
```

`extensions/v1beta1` derived the selector from the template labels, `apps/v1` wants it spelled out:

```yaml
spec:
  replicas: 2
  selector:
    matchLabels:
      app: reports
```

**hpa.yaml** — `apiVersion: autoscaling/v2`; the dry run then complains about the unknown field
`targetAverageUtilization`. `k explain hpa.spec.metrics.resource.target` shows the new shape. Also
fix the reference to the Deployment, which the server does not validate:

```yaml
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: reports
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
```

Apply and answer the question about the PodDisruptionBudget:

```bash
k apply -f .
k -n app-marmot get deploy,cronjob,hpa
echo policy/v1 > /opt/course/marmot/pdb-version
```

The HPA shows `cpu: <unknown>/70%` because this cluster has no metrics-server; that is fine here.

**Takeaway:** `k api-resources` (which group/version serves a kind), `k explain <kind>.<field>`
(what the current schema looks like) and `k apply --dry-run=server` (does the server accept my
file) migrate any manifest without the docs. Group `v1` kinds print just `v1`; all others are
`group/version`.
