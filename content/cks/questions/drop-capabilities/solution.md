```bash
ssh {{host}}
k -n sec-egret exec deploy/gateway -- grep CapEff: /proc/1/status     # 00000000aa8435fb or similar: far too much
k -n sec-egret edit deployment gateway
```

```yaml
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            drop: ["ALL"]
            add: ["CHOWN"]
```

**Verify**

```bash
k -n sec-egret rollout status deployment gateway
k -n sec-egret exec deploy/gateway -- grep CapEff: /proc/1/status     # CapEff: 0000000000000001
k -n sec-egret exec deploy/gateway -- wget -qO- http://127.0.0.1/     # gateway-ok
```

**What goes wrong**

- Only `drop: ["ALL"]`: the startup fails with `chown: /www: Operation not permitted`, the Pod crash-loops.
  `k -n sec-egret logs deploy/gateway` shows it. (Binding port 80 is not the problem: in a Pod, ports
  below 1024 need no capability, because the runtime sets `net.ipv4.ip_unprivileged_port_start=0`.)
- Keeping `add: [NET_ADMIN, SYS_TIME]` next to `drop: ["ALL"]`: added capabilities win over the drop.
  `NET_ADMIN` lets the container reconfigure interfaces and firewall rules of its network namespace.
- Writing `CAP_CHOWN`: Kubernetes expects the name without the prefix.
- Guessing instead of reading: the logs of the crashing container name the system call that was denied.
