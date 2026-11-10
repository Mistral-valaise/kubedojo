```bash
ssh {{host}}
k describe node {{host}}-control-plane | grep Taints
# Taints: node-role.kubernetes.io/control-plane:NoSchedule
k get node {{host}}-control-plane --show-labels | tr ',' '\n' | grep node-role
# node-role.kubernetes.io/control-plane=
k -n team-wader run cp-probe --image=busybox:1.36 $do --command -- sleep 1d > pod.yaml
vim pod.yaml
```

```yaml
spec:
  nodeSelector:
    node-role.kubernetes.io/control-plane: ""     # label exists with an empty value
  tolerations:
    - key: node-role.kubernetes.io/control-plane
      effect: NoSchedule
  containers:
    - name: cp-probe
      image: busybox:1.36
      command: ["sleep", "1d"]
```

```bash
k apply -f pod.yaml
k -n team-wader get pod -o wide           # NODE {{host}}-control-plane
```

| Only toleration | may land on the control plane **or** any worker |
|---|---|
| Only nodeSelector | `Pending`: the one matching node repels the Pod |
| Both | exactly the control plane node |
