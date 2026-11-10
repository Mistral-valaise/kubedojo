```bash
ssh {{host}}
sudo kubeadm token create --ttl 2h --print-join-command | tee /opt/course/join/join.sh
# kubeadm join {{host}}-control-plane:6443 --token 3k1q0d.8v6l2h9x4m7c5b1z --discovery-token-ca-cert-hash sha256:8f...
sudo kubeadm token list
# TOKEN                     TTL   EXPIRES                USAGES                   DESCRIPTION
# 3k1q0d.8v6l2h9x4m7c5b1z   1h    2026-11-10T23:58:11Z   authentication,signing   <none>
```

The command has three parts a joining node needs:

| Part | Purpose |
|---|---|
| `host:6443` | where the API server is |
| `--token` | lets the node authenticate for its first request (TLS bootstrap) |
| `--discovery-token-ca-cert-hash` | lets the node verify the cluster's CA |

On the new node you would run that line as root. For an additional **control plane** node the command also
needs `--control-plane --certificate-key ...`.
