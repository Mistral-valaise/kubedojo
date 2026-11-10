RBAC always combines three things: **who** (subject), **what** (Role with rules), and the **binding** between them.

```bash
ssh {{host}}

k -n team-orca create serviceaccount pipeline

k -n team-orca create role pipeline \
  --verb=create,list \
  --resource=deployments.apps,configmaps

k -n team-orca create rolebinding pipeline \
  --role=pipeline \
  --serviceaccount=team-orca:pipeline
```

Always verify RBAC with `auth can-i` — it is faster than reading YAML:

```bash
k -n team-orca auth can-i create deployments --as system:serviceaccount:team-orca:pipeline   # yes
k -n team-orca auth can-i delete deployments --as system:serviceaccount:team-orca:pipeline   # no
k -n default   auth can-i list configmaps    --as system:serviceaccount:team-orca:pipeline   # no
```

**Why Role + RoleBinding?** The four combinations:

| Role kind | Binding kind | Result |
|---|---|---|
| Role | RoleBinding | permissions in one Namespace |
| ClusterRole | RoleBinding | reusable rules, still limited to the binding's Namespace |
| ClusterRole | ClusterRoleBinding | permissions in all Namespaces |
| Role | ClusterRoleBinding | not possible |
