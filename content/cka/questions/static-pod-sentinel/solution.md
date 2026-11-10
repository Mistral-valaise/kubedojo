```bash
ssh {{host}}
sudo -i
grep staticPodPath /var/lib/kubelet/config.yaml     # staticPodPath: /etc/kubernetes/manifests

kubectl run sentinel --image=nginx:1.27-alpine --labels role=sentinel \
  --dry-run=client -o yaml > /etc/kubernetes/manifests/sentinel.yaml
vim /etc/kubernetes/manifests/sentinel.yaml
```

Add the requests:

```yaml
spec:
  containers:
  - image: nginx:1.27-alpine
    name: sentinel
    resources:
      requests:
        cpu: 10m
        memory: 20Mi
```

No `kubectl apply` — the kubelet picks the file up by itself:

```bash
kubectl get pod -o wide
# sentinel-{{host}}-control-plane   1/1   Running
```

Static Pods do not need the scheduler or the controller manager, which is exactly why the control plane
itself is run this way. To remove one, delete its file.
