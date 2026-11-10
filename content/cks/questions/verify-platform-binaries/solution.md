```bash
ssh {{host}}
cd /opt/course/binaries
cat kubectl.sha512
sha512sum kubectl-a kubectl-b kubectl-c
```

Do not compare 128 characters by eye. Either filter out the matching lines:

```bash
sha512sum kubectl-? | grep -v "$(cat kubectl.sha512)"
# 9c1e...  kubectl-b
```

or use the check mode of the tool (`checksum`, two spaces, `file name`):

```bash
for f in kubectl-?; do echo "$(cat kubectl.sha512)  $f" | sha512sum -c -; done
# kubectl-a: OK
# kubectl-b: FAILED
# kubectl-c: OK
```

```bash
echo kubectl-b > tampered
rm kubectl-b
```

**In real life** the checksum comes from the release page (`https://dl.k8s.io/release/<version>/bin/linux/amd64/kubectl.sha512`),
fetched over TLS separately from the binary. Verify `kubeadm`, `kubelet` and `kubectl` the same way before
installing them on a node.
