**1. The kubelet**

```bash
ssh {{worker}}
sudo -i
grep -n seccomp /var/lib/kubelet/config.yaml          # not set → Unconfined is the default
echo 'seccompDefault: true' >> /var/lib/kubelet/config.yaml
systemctl restart kubelet
systemctl is-active kubelet                           # active
exit; exit
```

**2. The workload**

The kubelet applies the default when it creates a container. The running one is still unconfined:

```bash
ssh {{host}}
k -n sec-avocet exec deploy/reports -- grep Seccomp: /proc/1/status    # Seccomp: 0
k -n sec-avocet rollout restart deployment reports
k -n sec-avocet rollout status deployment reports
k -n sec-avocet exec deploy/reports -- grep Seccomp: /proc/1/status    # Seccomp: 2
```

`0` is disabled, `1` is strict mode, `2` is filter mode — a profile is attached.

**Why the node and not the Deployment?** `securityContext.seccompProfile.type: RuntimeDefault` in a Pod
spec confines that Pod. `seccompDefault` confines every Pod on the node that does not say otherwise,
including the ones nobody reviewed. A Pod can still opt out with `type: Unconfined`; Pod Security
Standard *restricted* forbids that.
