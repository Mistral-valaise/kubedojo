# Cluster Architecture, Installation & Configuration

**25% of the exam.** RBAC, kubeadm lifecycle, etcd, certificates, Helm, Kustomize and CRDs.

## The 20% that solves 80%

### Know where everything lives (kubeadm cluster)

| What | Where |
|---|---|
| Static Pod manifests (apiserver, etcd, scheduler, controller-manager) | `/etc/kubernetes/manifests/` |
| Certificates and keys | `/etc/kubernetes/pki/` (etcd: `/etc/kubernetes/pki/etcd/`) |
| Admin kubeconfig | `/etc/kubernetes/admin.conf` |
| Kubelet config and flags | `/var/lib/kubelet/config.yaml`, `/var/lib/kubelet/kubeadm-flags.env` |
| Kubelet service | `systemctl status kubelet`, `journalctl -u kubelet` |
| CNI config | `/etc/cni/net.d/` |

Almost every "find out how the cluster is configured" task is answered by reading these files.

### RBAC

Four objects, two scopes. Create them imperatively and **always verify with `auth can-i`**.

```bash
k -n ns create serviceaccount app
k -n ns create role app --verb=get,list --resource=pods,deployments.apps
k -n ns create rolebinding app --role=app --serviceaccount=ns:app
k -n ns auth can-i list pods --as system:serviceaccount:ns:app
```

- `Role` + `RoleBinding` → one Namespace. `ClusterRole` + `ClusterRoleBinding` → everywhere.
- `ClusterRole` + `RoleBinding` → reusable rules, limited to the binding's Namespace.
- RBAC is additive only. There is no "deny".

### kubeadm: upgrade and join

Upgrade order is fixed: **control plane first, then workers; kubeadm first, then kubelet.**

```bash
# control plane
apt-get install -y kubeadm=1.xx.y-*          # 1. new kubeadm
kubeadm upgrade plan && kubeadm upgrade apply v1.xx.y
k drain <node> --ignore-daemonsets            # 2. drain
apt-get install -y kubelet=1.xx.y-* kubectl=1.xx.y-*
systemctl daemon-reload && systemctl restart kubelet
k uncordon <node>
# worker: same, but `kubeadm upgrade node` instead of plan/apply
```

Join a node: on the control plane `kubeadm token create --print-join-command`, run the output on the new node.

### etcd backup and restore

```bash
export ETCDCTL_API=3
etcdctl snapshot save /tmp/snap.db \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key
```

You do not need to memorise the flags: they are all in `/etc/kubernetes/manifests/etcd.yaml`.
Restore writes a **new data directory**; then point the etcd static Pod's `hostPath` volume at it:

```bash
etcdutl snapshot restore /tmp/snap.db --data-dir /var/lib/etcd-restore
vim /etc/kubernetes/manifests/etcd.yaml     # hostPath path: /var/lib/etcd-restore
```

### Certificates

```bash
kubeadm certs check-expiration
kubeadm certs renew apiserver
openssl x509 -in /etc/kubernetes/pki/apiserver.crt -noout -text | grep -A2 Validity
```

### Helm, Kustomize, CRDs

```bash
helm repo add <name> <url> && helm repo update
helm -n ns install rel repo/chart --set key=value        # or -f values.yaml
helm -n ns upgrade rel repo/chart --reuse-values --set replicas=3
helm -n ns ls -a ; helm -n ns rollback rel 1 ; helm show values repo/chart

k kustomize overlays/prod            # render
k apply -k overlays/prod             # render and apply

k get crd ; k explain <kind>.spec    # explain works for CRDs too
```

Operators need RBAC for the resources they manage — when an operator Pod logs `forbidden`,
extend its Role/ClusterRole.

## Habits that save points

- `k config get-contexts -o name`, `k config view --raw` — kubeconfig questions are free points.
- For "write the command into a file" tasks, write the command — not its output.
- Static Pod names always end with the node name: `kube-apiserver-<node>`.
