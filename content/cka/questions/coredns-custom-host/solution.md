```bash
ssh {{host}}
k -n kube-system get cm coredns -o jsonpath='{.data.Corefile}' > /opt/course/dns/Corefile.bak
k -n kube-system edit cm coredns
```

Add a `hosts` block inside the server block:

```text
.:53 {
    errors
    health {
       lameduck 5s
    }
    ready
    hosts {
        10.20.30.40 legacy-db.corp.internal
        fallthrough
    }
    kubernetes cluster.local in-addr.arpa ip6.arpa {
       pods insecure
       fallthrough in-addr.arpa ip6.arpa
       ttl 30
    }
    forward . /etc/resolv.conf {
       max_concurrent 1000
    }
    cache 30
    loop
    reload
    loadbalance
}
```

`fallthrough` is the important word: without it the `hosts` plugin answers every query itself and returns
NXDOMAIN for anything not in its list — the whole cluster would lose DNS.

Wait for the reload (up to a minute) or force it, then test all three kinds of names:

```bash
k -n kube-system rollout restart deploy coredns
k run tmp --rm -i --restart=Never --image=busybox:1.36 -- nslookup legacy-db.corp.internal
k run tmp --rm -i --restart=Never --image=busybox:1.36 -- nslookup kubernetes.default.svc.cluster.local
```

If CoreDNS Pods crash after the change: `k -n kube-system logs -l k8s-app=kube-dns` names the line of the
syntax error, and the backup restores the old state.
