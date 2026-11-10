```bash
ssh {{host}}
ssh {{worker}}
curl -sk https://localhost:10250/pods | head -c 200       # a full Pod list, without any credentials
sudo vim /var/lib/kubelet/config.yaml
```

```yaml
authentication:
  anonymous:
    enabled: false          # was: true
  webhook:
    cacheTTL: 0s
    enabled: true
  x509:
    clientCAFile: /etc/kubernetes/pki/ca.crt
authorization:
  mode: Webhook             # was: AlwaysAllow
```

```bash
sudo systemctl restart kubelet
systemctl status kubelet --no-pager | head -5
curl -sk https://localhost:10250/pods                     # Unauthorized
exit
k get nodes                                               # Ready
```

**Where CIS findings are fixed:**

| Component | File | Apply with |
|---|---|---|
| kubelet | `/var/lib/kubelet/config.yaml` | `systemctl restart kubelet` |
| API server, scheduler, controller manager, etcd | `/etc/kubernetes/manifests/*.yaml` | automatic (kubelet restarts the static Pod) |

`kube-bench run --targets node` prints the findings together with the remediation for each.
