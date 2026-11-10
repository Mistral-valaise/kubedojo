```bash
ssh {{host}}
mkdir -p /opt/course/kustomize/overlays/prod
vim /opt/course/kustomize/overlays/prod/kustomization.yaml
```

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ../../base
namespace: team-swift
namePrefix: prod-
replicas:
  - name: notifier        # the name as written in the base
    count: 3
labels:
  - pairs:
      env: prod
```

Render first, then apply:

```bash
k kustomize /opt/course/kustomize/overlays/prod          # check names, namespace, replicas, labels
k apply -k /opt/course/kustomize/overlays/prod
k -n team-swift get deploy,svc --show-labels
```

Kustomize rewrites references consistently: the Service is renamed to `prod-notifier`, and it still
selects the right Pods.

For changes without a built-in field (e.g. resource limits, an extra env var) an overlay uses `patches:`
with a strategic-merge or JSON patch.
