```bash
ssh {{host}}
helm -n app-hare ls
# NAME    NAMESPACE  REVISION  STATUS    CHART
# ledger  app-hare   3         deployed  ledger-1.4.0
k -n app-hare get deploy,pod
# ledger   3/3   1   3          one new Pod in ImagePullBackOff, the old ones still serve
```

Helm says `deployed`: without `--wait` it only records that the manifests were accepted, not that the
Pods came up. The history lists the revisions, the values tell them apart:

```bash
helm -n app-hare history ledger
helm -n app-hare get values ledger --revision 1      # null (chart defaults: 2 replicas)
helm -n app-hare get values ledger --revision 2      # replicaCount: 3
helm -n app-hare get values ledger --revision 3      # replicaCount: 3, image.tag: 1.72-alpine
```

**1. The broken image** is repository plus tag (`-a` also prints the defaults, here the repository):

```bash
helm -n app-hare get values ledger --revision 3 -a
echo "nginx:1.72-alpine" > /opt/course/hare/bad-image.txt
```

`k -n app-hare describe pod <pending pod>` shows the same image in its Events.

**2. Roll back** to revision 2, the last one that worked. Revision 1 works as well, but would shrink
the release to 2 replicas.

```bash
helm -n app-hare rollback ledger 2
helm -n app-hare history ledger          # revision 4: "Rollback to 2"
```

A rollback does not delete anything from the history; it adds a new revision with the old content.

**3. Upgrade** with the new log level. Find the value name first:

```bash
helm show values /opt/course/hare/ledger          # config.logLevel
helm -n app-hare upgrade ledger /opt/course/hare/ledger --reuse-values --set config.logLevel=warn
```

Without `--reuse-values` the upgrade would start from the chart defaults and silently reset
`replicaCount` to 2.

```bash
helm -n app-hare get values ledger                # replicaCount: 3, config.logLevel: warn, ...
k -n app-hare rollout status deploy/ledger
k -n app-hare exec deploy/ledger -- printenv LOG_LEVEL      # warn
```

**Takeaway:** `helm history` + `helm get values --revision N` answer "what changed when", `helm rollback
<release> <revision>` goes back, and every `helm upgrade` needs a decision about the old values
(`--reuse-values`, or pass them all again with `-f`).
