A Deployment with `0` Pods and no Pod in `Pending`: the Pods are rejected when the ReplicaSet tries
to create them. The reason is in the ReplicaSet's events, not in the Deployment.

```bash
ssh {{host}}
k -n app-rabbit get deploy,rs,pod
k -n app-rabbit describe rs -l app=worker | tail -5
#   Warning  FailedCreate  ...  Error creating: pods "worker-..." is forbidden:
#            maximum cpu usage per Container is 500m, but limit is 800m
k -n app-rabbit describe limitrange rabbit-limits
#   Type       Resource  Min   Max    Default Request  Default Limit
#   Container  cpu       100m  500m   500m             500m
k -n app-rabbit describe quota rabbit-quota
#   Resource      Used   Hard
#   limits.cpu    500m   2
#   requests.cpu  250m   1          <- db already takes 250m
```

Two rules, checked one after the other:

1. **LimitRange** (per container): CPU between `100m` and `500m`. The limit of `800m` is rejected.
2. **ResourceQuota** (sum of the Namespace): `1` CPU of requests, `250m` used by `db`. Three workers
   share `750m`: at most `250m` each. Lowering only the limit to `500m` and keeping the request of
   `400m` gets one Pod through and the next one fails with `exceeded quota: rabbit-quota`.

```bash
k -n app-rabbit set resources deploy worker --requests=cpu=200m --limits=cpu=400m
k -n app-rabbit rollout status deploy worker
k -n app-rabbit get pod -l app=worker                       # 3 Running
k -n app-rabbit describe quota rabbit-quota                 # requests.cpu  850m  1
echo 850m/1 > /opt/course/rabbit/cpu-requests
```

Any request from `100m` to `250m` with a limit up to `500m` is correct; the figure in the file is
whatever `describe quota` shows for your values.

**Takeaways**

- `FailedCreate` on the ReplicaSet = admission said no. `forbidden: maximum/minimum ... per Container`
  is the LimitRange, `forbidden: exceeded quota` is the ResourceQuota.
- A quota on `requests.cpu`/`limits.cpu` forces every Pod to have these values. Removing the
  `resources` block does not help here: the LimitRange would default the request to `500m`.
- Quota usage counts requests of all Pods that are not finished, also the ones that are Pending.
