```bash
ssh {{host}}
k -n team-dove get pod                         # one Pod in ImagePullBackOff, three old ones Running
k -n team-dove rollout history deploy gateway
# REVISION  CHANGE-CAUSE
# 1         <none>
# 2         <none>
k -n team-dove rollout undo deploy gateway     # back to revision 1 (stored as revision 3)
k -n team-dove rollout status deploy gateway
```

Set the strategy (`k -n team-dove edit deploy gateway`):

```yaml
spec:
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 0
      maxSurge: 1
```

Roll forward:

```bash
k -n team-dove set image deploy gateway nginx=nginx:1.27-alpine
k -n team-dove rollout status deploy gateway
```

**How to read the strategy:** with 4 replicas, `maxSurge: 1` and `maxUnavailable: 0` Kubernetes starts
one new Pod, waits until it is Ready, then removes one old Pod — and repeats. A broken image stops the
rollout at the first new Pod while all four old Pods keep serving.

`k rollout undo --to-revision=N` jumps to a specific revision;
`k rollout history deploy gateway --revision=N` shows its Pod template.
