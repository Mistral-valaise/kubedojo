The Pods run, so this is a **configuration** problem, not a crash. First reproduce it:

```bash
ssh {{host}}
k run tmp --rm -i --restart=Never --image=busybox:1.36 -- nslookup kubernetes.default.svc.cluster.local
# ** server can't find kubernetes.default.svc.cluster.local: NXDOMAIN
```

CoreDNS has exactly one place for configuration:

```bash
k -n kube-system get cm coredns -o yaml
```

```text
.:53 {
    errors
    health
    kubernetes cluster.lokal in-addr.arpa ip6.arpa {     # <-- typo
       pods insecure
       fallthrough in-addr.arpa ip6.arpa
    }
    forward . /etc/resolv.conf
    cache 30
    reload
}
```

The `kubernetes` plugin only answers for the zones named on its line. Fix the zone:

```bash
k -n kube-system edit cm coredns          # kubernetes cluster.local in-addr.arpa ip6.arpa {
```

The `reload` plugin notices the change after up to a minute. To apply it immediately:

```bash
k -n kube-system rollout restart deploy coredns
```

Verify:

```bash
k run tmp --rm -i --restart=Never --image=busybox:1.36 -- nslookup kubernetes.default.svc.cluster.local
```

**DNS debugging order:** CoreDNS Pods running? → Service `kube-dns` has endpoints? → Corefile correct?
→ Pod's `/etc/resolv.conf` points at the `kube-dns` ClusterIP?
