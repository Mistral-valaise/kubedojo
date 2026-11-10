```bash
ssh {{host}}
sudo -i
cp /etc/kubernetes/manifests/kube-apiserver.yaml /root/kube-apiserver.yaml.bak
grep -E 'admission-plugins|profiling' /etc/kubernetes/manifests/kube-apiserver.yaml
#    - --enable-admission-plugins=NamespaceLifecycle
#    - --profiling=true
vim /etc/kubernetes/manifests/kube-apiserver.yaml
```

```yaml
    - --enable-admission-plugins=NodeRestriction
    - --profiling=false
```

(Keeping other plugins in the list is fine: `NamespaceLifecycle,NodeRestriction`.)

Wait for the restart and verify the **running process**, not just the file:

```bash
watch crictl ps --name kube-apiserver
ps aux | grep kube-apiserver | tr ' ' '\n' | grep -E 'admission-plugins|profiling'
kubectl get --raw /readyz        # ok
```

**If the API server does not come back:**

```bash
crictl ps -a --name kube-apiserver          # Exited container?
crictl logs <container-id>                  # e.g. "unknown flag" or "invalid plugin name"
journalctl -u kubelet | grep -i apiserver   # YAML syntax errors appear here
cp /root/kube-apiserver.yaml.bak /etc/kubernetes/manifests/kube-apiserver.yaml   # last resort
```

Never store the backup inside `/etc/kubernetes/manifests` — the kubelet would try to run it as a second
API server.
