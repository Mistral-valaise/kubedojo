```bash
ssh {{host}}
k -n app-seal logs deploy/job-watcher --tail=2                  # jobs: access denied
k -n app-seal get deploy job-watcher -o jsonpath='{.spec.template.spec.serviceAccountName}'   # empty = default
k api-resources | grep -i job                                   # jobs  batch/v1  -> group "batch"
```

Three objects, all imperative:

```bash
k -n app-seal create serviceaccount job-watcher
k -n app-seal create role job-reader --verb=get,list,watch --resource=jobs.batch
k -n app-seal create rolebinding job-watcher-reads-jobs \
  --role=job-reader --serviceaccount=app-seal:job-watcher
```

Test the rule before touching the workload, the positive and the negative:

```bash
SA=system:serviceaccount:app-seal:job-watcher
k -n app-seal auth can-i list jobs.batch --as $SA        # yes
k -n app-seal auth can-i delete jobs.batch --as $SA      # no
k -n app-seal auth can-i list pods --as $SA              # no
k -n default  auth can-i list jobs.batch --as $SA        # no
```

Now give the Pods the identity. `serviceAccountName` is part of the Pod template, so this is a rollout:

```bash
k -n app-seal set serviceaccount deploy job-watcher job-watcher
k -n app-seal rollout status deploy job-watcher
k -n app-seal logs deploy/job-watcher --tail=2                  # jobs: access ok
```

The same as YAML:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: {name: job-reader, namespace: app-seal}
rules:
  - apiGroups: ["batch"]            # not "" and not "batch/v1"
    resources: ["jobs"]             # plural, lower case
    verbs: ["get", "list", "watch"]
```

**Takeaways**

- ServiceAccount = who, Role = what, RoleBinding = who gets what, and the Pod must name the account.
  Four places; the error stays until all four are right.
- The subject of a ServiceAccount is `system:serviceaccount:<namespace>:<name>`; with `--serviceaccount`
  it is `<namespace>:<name>`.
- Do not "fix" it by granting the Role to `default`: every Pod of the Namespace without an explicit
  ServiceAccount would inherit the permission.
