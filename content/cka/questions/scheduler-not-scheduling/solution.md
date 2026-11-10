`Pending` with **no events** is the signature of a missing scheduler. (`Pending` *with* a `FailedScheduling`
event means the scheduler works but found no fitting node.)

```bash
ssh {{host}}
k -n team-ibis describe pod reports        # Events: <none>
k -n kube-system get pod                   # kube-scheduler-... CrashLoopBackOff
```

Read the container log. If `kubectl logs` is not available, use the runtime directly:

```bash
sudo -i
crictl ps -a | grep kube-scheduler
crictl logs <container-id>
# ... failed to get delegated authentication kubeconfig: stat /etc/kubernetes/scheduler.config: no such file or directory
ls /etc/kubernetes/*.conf                  # scheduler.conf exists
```

Fix the static Pod manifest. The kubelet watches the directory and restarts the Pod on its own:

```bash
vim /etc/kubernetes/manifests/kube-scheduler.yaml
#    - --kubeconfig=/etc/kubernetes/scheduler.conf
watch crictl ps                            # wait for kube-scheduler to come up
k -n team-ibis get pod reports             # Running
```

**Symptom → component:**

| Symptom | Suspect |
|---|---|
| Pods `Pending`, no events | kube-scheduler |
| Deployments do not create Pods, Namespaces hang in `Terminating` | kube-controller-manager |
| `kubectl` gets connection refused | kube-apiserver or etcd |
