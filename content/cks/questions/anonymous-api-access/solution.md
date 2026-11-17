A request without credentials is not rejected by default: it runs as user `system:anonymous` in group
`system:unauthenticated`, and RBAC decides what that identity may do. So look for bindings with one of
these two subjects.

```bash
ssh {{host}}
k get clusterrolebindings -o json | jq -r '
  .items[] | select(any(.subjects[]?; .name=="system:anonymous" or .name=="system:unauthenticated"))
  | "\(.metadata.name)\t\(.roleRef.name)"'
# kd-legacy-dashboard        view
# kd-node-inventory          kd-node-inventory
# system:public-info-viewer  system:public-info-viewer
k get rolebindings -A -o json | jq -r '
  .items[] | select(any(.subjects[]?; .name=="system:anonymous" or .name=="system:unauthenticated"))
  | "\(.metadata.namespace)/\(.metadata.name)"'
# kube-public/kubeadm:bootstrap-signer-clusterinfo   (kubeadm join needs it)
```

`system:public-info-viewer` is built in and only covers `/healthz`, `/livez`, `/readyz` and `/version`.
The other two are the problem:

```bash
k delete clusterrolebinding kd-legacy-dashboard kd-node-inventory
printf 'kd-legacy-dashboard\nkd-node-inventory\n' > /opt/course/anonymous/removed
```

**Verify**

```bash
k auth can-i list configmaps -n kube-system --as system:anonymous --as-group system:unauthenticated   # no
k auth can-i list nodes --as system:anonymous --as-group system:unauthenticated                       # no
curl -sk https://127.0.0.1:6443/version | head -3                                                     # still answers
```

**Why not `--anonymous-auth=false`?** It would close the health endpoints too, and the kubelet probes
the API server's `/livez` and `/readyz` without credentials — the static Pod would be restarted over
and over. Fix the authorization, not the authentication.
