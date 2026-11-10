```bash
ssh {{host}}
k -n team-ruff create deploy ledger-sync --image=nginx:1.27-alpine --replicas=3 $do > deploy.yaml
vim deploy.yaml
```

**Variant 1 — pod anti-affinity** ("do not place me on a node that already runs a Pod like me"):

```yaml
    spec:
      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            - labelSelector:
                matchLabels:
                  app: ledger-sync
              topologyKey: kubernetes.io/hostname
```

**Variant 2 — topology spread constraints** ("keep the difference between nodes at most 1"):

```yaml
    spec:
      topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: kubernetes.io/hostname
          whenUnsatisfiable: DoNotSchedule
          labelSelector:
            matchLabels:
              app: ledger-sync
```

```bash
k apply -f deploy.yaml
k -n team-ruff get pod -o wide
# ledger-sync-...   1/1   Running   {{worker}}
# ledger-sync-...   1/1   Running   {{worker2}}
# ledger-sync-...   0/1   Pending   <none>
k -n team-ruff describe pod <pending-pod> | tail -3
# 0/3 nodes are available: 1 node(s) had untolerated taint ..., 2 node(s) didn't match pod anti-affinity rules.
```

`topologyKey` defines what "together" means: `kubernetes.io/hostname` = same node,
`topology.kubernetes.io/zone` = same zone.
