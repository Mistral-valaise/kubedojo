```bash
ssh {{host}}
helm -n team-otter ls
# NAME        NAMESPACE    REVISION  STATUS    CHART
# old-cache   team-otter   1         deployed  webcache-0.3.1
helm -n team-otter uninstall old-cache
```

Find out what the chart lets you configure:

```bash
helm show values /opt/course/helm/webcache
# replicaCount: 1
# image: {repository: nginx, tag: 1.27-alpine}
# service: {type: ClusterIP, port: 80}
```

Install with overrides:

```bash
helm -n team-otter install cache /opt/course/helm/webcache \
  --set replicaCount=2 \
  --set service.type=NodePort
helm -n team-otter ls
k -n team-otter get deploy,svc
```

**The Helm commands worth knowing by heart:**

| Command | Purpose |
|---|---|
| `helm ls -A` | releases in all Namespaces (`-a` includes failed/pending) |
| `helm show values <chart>` | configurable values of a chart |
| `helm get values <release>` | values set on a release |
| `helm upgrade <release> <chart> --reuse-values --set k=v` | change a release |
| `helm rollback <release> <revision>` | go back |
| `helm repo add / update`, `helm search repo` | work with chart repositories |
