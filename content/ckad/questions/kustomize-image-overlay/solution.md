```bash
ssh {{host}}
cat /opt/course/jackal/base/*.yaml          # image nginx:1.26, 1 replica, envFrom ConfigMap reporter-config
mkdir -p /opt/course/jackal/overlays/staging
vim /opt/course/jackal/overlays/staging/kustomization.yaml
```

The base references a ConfigMap `reporter-config` that it does not contain: every environment brings
its own. Each requirement has a built-in Kustomize field:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ../../base
namespace: app-jackal
namePrefix: stg-
labels:
  - pairs:
      stage: staging
images:
  - name: nginx                 # the image name as written in the base, without tag
    newTag: 1.27-alpine
replicas:
  - name: reporter              # the Deployment name as written in the base, without prefix
    count: 2
configMapGenerator:
  - name: reporter-config       # the name the base Deployment refers to
    literals:
      - REPORT_INTERVAL=30s
```

Render first, then apply:

```bash
k kustomize /opt/course/jackal/overlays/staging
#   kind: ConfigMap   name: stg-reporter-config-7f8k2m9c4h
#   kind: Deployment  name: stg-reporter  image: nginx:1.27-alpine  replicas: 2
#       envFrom: configMapRef: name: stg-reporter-config-7f8k2m9c4h
k apply -k /opt/course/jackal/overlays/staging
k -n app-jackal get deploy,svc,cm --show-labels
k -n app-jackal exec deploy/stg-reporter -- printenv REPORT_INTERVAL      # 30s
```

Kustomize appends a hash of the content to the name of a generated ConfigMap and rewrites the
reference in the Deployment. When the configuration changes, the name changes, the Pod template
changes with it, and `apply -k` rolls the Pods: no forgotten restart after a config change.

**Takeaway:** `images`, `replicas`, `labels`, `namePrefix`, `namespace` and `configMapGenerator` cover
most overlay tasks without writing a patch. In `images` and `replicas` you always use the names from
the base, not the prefixed ones.
