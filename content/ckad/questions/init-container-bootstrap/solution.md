```bash
ssh {{host}}
k -n app-cougar edit deploy docs
```

Three additions to the Pod template (`spec.template.spec`):

```yaml
    spec:
      volumes:                                  # 1. the shared volume
        - name: site
          emptyDir: {}
      initContainers:                           # 2. runs first, must exit 0
        - name: fetch-content
          image: busybox:1.36
          command: ["sh", "-c", "echo \"cougar docs build 7 on $(hostname)\" > /site/index.html"]
          volumeMounts:
            - name: site
              mountPath: /site
      containers:
        - name: web
          image: nginx:1.27-alpine
          volumeMounts:                         # 3. the same volume where nginx reads
            - name: site
              mountPath: /usr/share/nginx/html
```

```bash
k -n app-cougar rollout status deploy docs
k -n app-cougar get pod                        # Init:0/1 -> PodInitializing -> Running
curl -s $(k -n app-cougar get svc docs -o jsonpath='{.spec.clusterIP}')
# cougar docs build 7 on docs-5c9d8f7b6-x2x7q
```

If a Pod hangs in `Init:Error` or `Init:CrashLoopBackOff`, read the init container's log:
`k -n app-cougar logs <pod> -c fetch-content`.

**Takeaways:**

- Init containers run one after another, each to completion, before any app container starts. Their
  filesystem is gone afterwards; only volumes carry their work over.
- The two containers may mount the same volume at different paths.
- A volume mounted over a directory hides what the image had there, which is why the nginx default
  page disappears.
- In `kubectl get pod`, `Init:0/1` means "waiting for init containers", and the app container's log
  is empty until they are done.
