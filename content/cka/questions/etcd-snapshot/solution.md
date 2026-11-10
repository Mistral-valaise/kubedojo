Do not memorise the flags — read them from the manifest:

```bash
ssh {{host}}
sudo -i
grep -E 'data-dir|listen-client|cert-file|key-file|trusted-ca' /etc/kubernetes/manifests/etcd.yaml
#    - --cert-file=/etc/kubernetes/pki/etcd/server.crt
#    - --data-dir=/var/lib/etcd
#    - --key-file=/etc/kubernetes/pki/etcd/server.key
#    - --listen-client-urls=https://127.0.0.1:2379,https://172.18.0.3:2379
#    - --trusted-ca-file=/etc/kubernetes/pki/etcd/ca.crt
```

```bash
ETCDCTL_API=3 etcdctl \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key \
  snapshot save /opt/course/etcd/snapshot.db

etcdutl snapshot status /opt/course/etcd/snapshot.db -w table     # verify
echo /var/lib/etcd > /opt/course/etcd/data-dir
```

**Restore, for completeness** (not asked here):

```bash
etcdutl snapshot restore /opt/course/etcd/snapshot.db --data-dir /var/lib/etcd-restored
vim /etc/kubernetes/manifests/etcd.yaml      # volumes: hostPath.path: /var/lib/etcd-restored
```

The kubelet then restarts etcd on the restored data.
