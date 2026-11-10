All answers are in files on the control plane node.

```bash
ssh {{host}}
sudo -i
cd /etc/kubernetes/manifests

grep service-cluster-ip-range kube-apiserver.yaml
#    - --service-cluster-ip-range=10.96.0.0/16
grep cluster-cidr kube-controller-manager.yaml
#    - --cluster-cidr=10.244.0.0/16
grep authorization-mode kube-apiserver.yaml
#    - --authorization-mode=Node,RBAC
ls /etc/cni/net.d/
# 10-kindnet.conflist
```

```bash
cd /opt/course/facts
echo 10.96.0.0/16        > service-cidr
echo 10.244.0.0/16       > pod-cidr
echo Node,RBAC           > authorization-modes
echo 10-kindnet.conflist > cni-config
```

(Use the values **your** cluster shows.)

**Which component owns which setting?**

| Setting | Component |
|---|---|
| Service IPs, authentication, authorization, admission | kube-apiserver |
| Pod CIDR per node, signing certificates, controllers | kube-controller-manager |
| Pod networking implementation | CNI plugin, configured in `/etc/cni/net.d` |
| Service → Pod traffic rules | kube-proxy (ConfigMap `kube-proxy` in `kube-system`) |
