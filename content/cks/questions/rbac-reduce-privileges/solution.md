**Find the source of the permissions.**

```bash
ssh {{host}}
k auth can-i --list --as system:serviceaccount:sec-heron:report-reader | head -4
# *.*   []   []   [*]            <- everything
k get clusterrolebinding -o wide | grep report-reader
# report-reader-binding   ClusterRole/cluster-admin   ...   sec-heron/report-reader
k get rolebinding -A -o wide | grep report-reader         # nothing else
```

**Remove it** — the binding, not the role:

```bash
k delete clusterrolebinding report-reader-binding
```

**Grant what is needed:**

```bash
k -n sec-heron create role report-reader --verb=get,list --resource=pods,configmaps
k -n sec-heron create rolebinding report-reader --role=report-reader \
  --serviceaccount=sec-heron:report-reader
```

**Verify both sides:**

```bash
SA=system:serviceaccount:sec-heron:report-reader
k -n sec-heron   auth can-i list pods   --as $SA      # yes
k -n sec-heron   auth can-i get secrets --as $SA      # no
k -n kube-system auth can-i list pods   --as $SA      # no
```

**What to look for in an RBAC review:** bindings to `cluster-admin`, `*` in verbs or resources,
`secrets`, `pods/exec`, and subjects like `system:authenticated` or the `default` ServiceAccount.
