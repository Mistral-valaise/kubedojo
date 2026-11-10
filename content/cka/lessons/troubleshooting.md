# Troubleshooting

**30% of the exam.** The largest domain — and the one where a fixed routine beats knowledge.

## The 20% that solves 80%

Always walk the same path, from the outside in. Do not guess; let the cluster tell you.

```bash
k get nodes                               # is the cluster itself healthy?
k -n <ns> get deploy,pod,svc,endpointslices
k -n <ns> describe pod <pod>              # read the Events at the bottom
k -n <ns> logs <pod> [-c container] [--previous]
k get events -A --sort-by=.lastTimestamp | tail -20
```

### Pod status tells you where to look

| Status | Meaning | Look at |
|---|---|---|
| `Pending` | not scheduled | `describe pod` → resources, taints, nodeSelector, unbound PVC |
| `ImagePullBackOff` | image cannot be pulled | image name/tag, registry, pull secret |
| `CrashLoopBackOff` | container starts, then exits | `logs --previous`, command/args, config, probes |
| `CreateContainerConfigError` | missing ConfigMap/Secret/key | `describe pod` |
| `Running` but not Ready | readiness probe fails | probe path/port, the app itself |
| `Terminating` forever | finalizer or dead node | `k get pod -o yaml`, node state |

### Service does not answer

1. `k get endpointslices -l kubernetes.io/service-name=<svc>` — **no endpoints** means the selector does not
   match Pod labels, or the Pods are not Ready.
2. Endpoints exist but no answer → wrong `targetPort`, or a NetworkPolicy blocks the traffic.
3. Name does not resolve → CoreDNS (`k -n kube-system get pod -l k8s-app=kube-dns`, then its logs and ConfigMap).

Test from inside the cluster, never from your laptop's point of view:

```bash
k run tmp --rm -i --restart=Never --image=busybox:1.36 -- wget -qO- -T 3 <svc>.<ns>
k run tmp --rm -i --restart=Never --image=busybox:1.36 -- nslookup <svc>.<ns>
```

### Node is NotReady

The kubelet is a **systemd service**, not a Pod. On the node:

```bash
ssh <node>; sudo -i
systemctl status kubelet
journalctl -u kubelet --no-pager | tail -30
```

Typical causes: service stopped/disabled, wrong path or flag in
`/usr/lib/systemd/system/kubelet.service.d/10-kubeadm.conf` or `/var/lib/kubelet/kubeadm-flags.env`,
broken `/var/lib/kubelet/config.yaml`, container runtime down (`systemctl status containerd`).
After changing a unit file: `systemctl daemon-reload && systemctl restart kubelet`.

### Control plane component is down

API server, scheduler, controller-manager and etcd are **static Pods** — the kubelet runs whatever it finds
in `/etc/kubernetes/manifests/`. If `kubectl` itself stops working, go below it:

```bash
crictl ps -a | grep kube-apiserver        # is the container there? restarting?
crictl logs <container-id>
ls /var/log/pods/ /var/log/containers/
journalctl -u kubelet | grep -i apiserver # YAML errors in the manifest show up here
```

Symptoms map to components: Pods stay `Pending` with no events → **scheduler**. Deployments create no
Pods → **controller-manager**. Nothing works → **apiserver** or **etcd**.

## Habits that save points

- Read the **whole** error line. Kubernetes error messages are usually exact.
- Before editing a static Pod manifest, copy it away: `cp /etc/kubernetes/manifests/x.yaml /root/x.yaml.bak`
  (not inside the manifests directory — the kubelet would run the copy).
- After a fix, wait and verify. The kubelet needs up to a minute to restart a static Pod.
- Fix in place (`edit`, `set image`, `patch`). Recreate only if the task allows it.
