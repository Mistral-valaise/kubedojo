set -e
ETCDCTL_API=3 etcdctl --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt --cert=/etc/kubernetes/pki/etcd/server.crt --key=/etc/kubernetes/pki/etcd/server.key \
  snapshot save /opt/course/etcd/snapshot.db
grep -o -- '--data-dir=[^ ]*' /etc/kubernetes/manifests/etcd.yaml | cut -d= -f2 > /opt/course/etcd/data-dir
