`Pending` **with** a `FailedScheduling` event: the scheduler works and tells you exactly why no node fits.

```bash
ssh {{host}}
k -n team-finch describe pod -l app=indexer | tail -4
# 0/3 nodes are available: 1 node(s) had untolerated taint {node-role.kubernetes.io/control-plane: },
# 2 node(s) didn't match Pod's node affinity/selector.
```

No node carries the label the Pod asks for:

```bash
k get nodes -L disk
k label node {{worker2}} disk=ssd
k -n team-finch describe pod -l app=indexer | tail -4
# 0/3 nodes are available: 1 Insufficient cpu, ...
```

Now the selector matches, and the next reason appears: the request is `64` CPUs.

```bash
k -n team-finch set resources deploy indexer --requests=cpu=100m
k -n team-finch get pod -o wide            # Running on {{worker2}}
```

**Reading the scheduler message:** it always starts with `0/N nodes are available` followed by one reason
per group of nodes. Work through the reasons until none is left.
