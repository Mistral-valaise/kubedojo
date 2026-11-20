```bash
ssh {{host}}
vim /opt/course/static/deployment.yaml
```

Four changes:

```diff
     spec:
-      hostNetwork: true
       containers:
         - name: exporter
-          image: nginx:latest
+          image: nginx:1.27-alpine
           securityContext:
-            privileged: true
+            privileged: false
+            allowPrivilegeEscalation: false
           volumeMounts:
-            - name: runtime
-              mountPath: /run/containerd/containerd.sock
             - name: cache
               mountPath: /var/cache/nginx
       volumes:
-        - name: runtime
-          hostPath:
-            path: /run/containerd/containerd.sock
         - name: cache
           emptyDir: {}
```

**Verify**

```bash
k apply --dry-run=client -f /opt/course/static/deployment.yaml      # deployment.apps/exporter created (dry run)
grep -nE 'privileged|hostNetwork|hostPath|image:|allowPrivilegeEscalation' /opt/course/static/deployment.yaml
```

**Typical mistakes**

- Removing the `hostPath` volume but leaving its `volumeMount` — the manifest is then invalid.
- Deleting `privileged: true` and stopping there: `allowPrivilegeEscalation` still defaults to `true`.
- Tools such as kubesec, KubeLinter or `trivy config` report these findings automatically; in the exam
  you are usually given the report and fix the manifest by hand, as here.
