A `NotReady` node almost always means: **the kubelet on that node is not running or cannot reach the API server.**
So go to the node.

```bash
ssh {{host}}
k get nodes                       # {{worker}}   NotReady
ssh {{worker}}
sudo -i
systemctl status kubelet          # activating (auto-restart) ... status=203/EXEC
journalctl -u kubelet --no-pager | tail -5
# kubelet.service: Failed to locate executable /usr/local/bin/kubelet: No such file or directory
```

`203/EXEC` means systemd could not execute the binary. Where is the command defined?

```bash
systemctl cat kubelet             # shows the unit and the drop-in with its path
which kubelet                     # /usr/bin/kubelet
vim /etc/systemd/system/kubelet.service.d/10-kubeadm.conf
# ExecStart=/usr/bin/kubelet $KUBELET_KUBECONFIG_ARGS ...
systemctl daemon-reload
systemctl restart kubelet
systemctl status kubelet          # active (running)
```

Back on the control plane, the node becomes `Ready` within a few seconds:

```bash
exit; exit
k get nodes
```

**Routine for kubelet problems:** `systemctl status` → `journalctl -u kubelet` → fix the file the error names →
`daemon-reload` (only needed for unit files) → `restart`.
