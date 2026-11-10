**1. The configuration**

```bash
ssh {{host}}
sudo -i
mkdir -p /etc/kubernetes/enc
head -c 32 /dev/urandom | base64          # the key
vim /etc/kubernetes/enc/enc.yaml
```

```yaml
apiVersion: apiserver.config.k8s.io/v1
kind: EncryptionConfiguration
resources:
  - resources:
      - secrets
    providers:
      - aescbc:
          keys:
            - name: key1
              secret: <the base64 key>
      - identity: {}
```

**2. The API server** — flag, volume mount and volume:

```bash
cp /etc/kubernetes/manifests/kube-apiserver.yaml /root/kube-apiserver.yaml.bak
vim /etc/kubernetes/manifests/kube-apiserver.yaml
```

```yaml
spec:
  containers:
  - command:
    - kube-apiserver
    - --encryption-provider-config=/etc/kubernetes/enc/enc.yaml
    ...
    volumeMounts:
    - mountPath: /etc/kubernetes/enc
      name: enc
      readOnly: true
    ...
  volumes:
  - hostPath:
      path: /etc/kubernetes/enc
      type: DirectoryOrCreate
    name: enc
```

```bash
watch crictl ps --name kube-apiserver      # wait until it is running again
```

**3. Encrypt the existing Secret** by writing it again:

```bash
kubectl -n sec-pelican get secret db-pass -o json | kubectl replace -f -
```

**Verify in etcd:**

```bash
ETCDCTL_API=3 etcdctl --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key \
  get /registry/secrets/sec-pelican/db-pass | hexdump -C | head -3
# 00000000  2f 72 65 67 69 73 74 72  79 2f 73 65 63 72 65 74  |/registry/secret|
# 00000020  ... 6b 38 73 3a 65 6e 63 3a  61 65 73 63 62 63 3a 76  |k8s:enc:aescbc:v|
```

Before the change the same command shows the password in plain text.

To encrypt **all** Secrets: `kubectl get secrets -A -o json | kubectl replace -f -`.
To rotate a key: add the new key first in the list, restart, rewrite all Secrets, then remove the old key.
