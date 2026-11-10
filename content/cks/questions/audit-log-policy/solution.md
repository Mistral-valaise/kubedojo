**1. The policy**

```bash
ssh {{host}}
sudo -i
mkdir -p /etc/kubernetes/audit
vim /etc/kubernetes/audit/policy.yaml
```

```yaml
apiVersion: audit.k8s.io/v1
kind: Policy
rules:
  - level: RequestResponse
    resources:
      - group: ""
        resources: ["pods"]
  - level: Metadata
    resources:
      - group: ""
        resources: ["secrets", "configmaps"]
  - level: None
```

**2. The API server**

```bash
cp /etc/kubernetes/manifests/kube-apiserver.yaml /root/kube-apiserver.yaml.bak
vim /etc/kubernetes/manifests/kube-apiserver.yaml
```

```yaml
    - kube-apiserver
    - --audit-policy-file=/etc/kubernetes/audit/policy.yaml
    - --audit-log-path=/var/log/kubernetes/audit/audit.log
    - --audit-log-maxage=7
    - --audit-log-maxbackup=2
    ...
    volumeMounts:
    - mountPath: /etc/kubernetes/audit/policy.yaml
      name: audit-policy
      readOnly: true
    - mountPath: /var/log/kubernetes/audit
      name: audit-log
    ...
  volumes:
  - hostPath:
      path: /etc/kubernetes/audit/policy.yaml
      type: File
    name: audit-policy
  - hostPath:
      path: /var/log/kubernetes/audit
      type: DirectoryOrCreate
    name: audit-log
```

**3. Verify**

```bash
watch crictl ps --name kube-apiserver
kubectl -n kube-system get secrets >/dev/null
tail -n 3 /var/log/kubernetes/audit/audit.log | jq -c '[.level, .verb, .objectRef.resource, .user.username]'
# ["Metadata","list","secrets","kubernetes-admin"]
```

**When the API server does not come back** it is almost always a mount: `crictl logs` of the exited
container says `open /etc/kubernetes/audit/policy.yaml: no such file or directory`.

**Reading the log:** one JSON object per line and stage. Useful fields: `level`, `stage`, `verb`,
`user.username`, `sourceIPs`, `objectRef.{resource,namespace,name}`, `responseStatus.code`.
