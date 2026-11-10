First learn what the CRD added to the API:

```bash
ssh {{host}}
k api-resources | grep -i backup
# backups   bkp   ops.kubedojo.dev/v1   true   Backup
k explain backup.spec
```

**RBAC.** The resource is `backups` in group `ops.kubedojo.dev`:

```bash
k create clusterrole backup-operator \
  --verb=get,list,watch,update \
  --resource=backups.ops.kubedojo.dev

k -n team-egret create rolebinding backup-operator \
  --clusterrole=backup-operator \
  --serviceaccount=team-egret:backup-operator

k -n team-egret auth can-i update backups --as system:serviceaccount:team-egret:backup-operator   # yes
k -n default    auth can-i list backups   --as system:serviceaccount:team-egret:backup-operator   # no
```

**The custom resource.** There is no generator; write it from `apiVersion` (group/version) and `kind`:

```yaml
apiVersion: ops.kubedojo.dev/v1
kind: Backup
metadata:
  name: nightly
  namespace: team-egret
spec:
  schedule: "0 2 * * *"
  retentionDays: 14
```

```bash
k apply -f backup.yaml
k -n team-egret get backups
```

Quote the cron expression — unquoted, `*` has a special meaning in YAML.
