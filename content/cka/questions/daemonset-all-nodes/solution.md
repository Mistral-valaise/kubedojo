```bash
ssh {{host}}
k -n team-owl create deploy node-agent --image=busybox:1.36 $do -- sleep 1d > ds.yaml
vim ds.yaml
```

Turn the Deployment into a DaemonSet: change `kind`, delete `replicas`, `strategy` and `status`, then
add requests and the toleration:

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: node-agent
  namespace: team-owl
spec:
  selector:
    matchLabels:
      app: node-agent
  template:
    metadata:
      labels:
        app: node-agent
    spec:
      tolerations:
        - key: node-role.kubernetes.io/control-plane
          effect: NoSchedule
      containers:
        - name: busybox
          image: busybox:1.36
          command: ["sleep", "1d"]
          resources:
            requests:
              cpu: 10m
              memory: 16Mi
```

```bash
k apply -f ds.yaml
k -n team-owl get ds,pod -o wide        # DESIRED = number of nodes, one Pod on each
```

Without the toleration `DESIRED` would be the number of worker nodes only: the DaemonSet controller
simply skips nodes whose taints the Pods do not tolerate.
