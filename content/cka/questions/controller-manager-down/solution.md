A Deployment without a ReplicaSet means nobody is reconciling Deployments — that is the job of the
**kube-controller-manager**.

```bash
ssh {{host}}
k -n team-stork get deploy,rs,pod          # deployment 0/2, nothing else
k -n kube-system get pod                   # kube-controller-manager-... CrashLoopBackOff
k -n kube-system logs kube-controller-manager-{{host}}-control-plane | tail -3
# ... stat /etc/kubernetes/controller-manger.conf: no such file or directory
```

The file name is misspelled (`manger`). Fix the static Pod manifest:

```bash
sudo -i
ls /etc/kubernetes/*.conf
vim /etc/kubernetes/manifests/kube-controller-manager.yaml
#    - --kubeconfig=/etc/kubernetes/controller-manager.conf
```

The kubelet restarts the Pod. Shortly after, the controller creates the ReplicaSet:

```bash
k -n kube-system get pod -l component=kube-controller-manager
k -n team-stork get deploy,rs
```

Note that the Pods of `ghost` may still not start if other parts of this cluster are broken — the task
only asks for the controller to do its work again.
