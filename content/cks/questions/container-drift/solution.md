**1. Find the Pod that differs**

```bash
ssh {{host}}
for p in $(k -n sec-dunlin get pod -o name); do echo "== $p"; k -n sec-dunlin exec $p -- ps; done
# one of them has extra lines:   23 root   0:00 {kthrotld} /bin/sh /bin/kthrotld
k -n sec-dunlin exec <pod> -- ls -l /bin/kthrotld              # a script, written minutes ago
k -n sec-dunlin exec <other pod> -- ls -l /bin/kthrotld        # No such file: not part of the image
echo <pod> > /opt/course/drift/pod
```

The name imitates a kernel thread. Kernel threads never show up inside a container's PID namespace,
and they have no file under `/bin` — this one is a disguise.

**2. Replace it**

```bash
k -n sec-dunlin delete pod <pod>
```

**3. Make the container immutable**

```bash
k -n sec-dunlin edit deployment ledger
```

```yaml
    spec:
      containers:
        - name: ledger
          securityContext:
            readOnlyRootFilesystem: true
          volumeMounts:
            - name: data
              mountPath: /data
      volumes:
        - name: data
          emptyDir: {}
```

**Verify**

```bash
k -n sec-dunlin rollout status deployment ledger
k -n sec-dunlin exec deploy/ledger -- sh -c 'cp /bin/busybox /bin/x'    # Read-only file system
k -n sec-dunlin exec deploy/ledger -- cat /data/heartbeat               # still written
```

Without the volume the application cannot write `/data/heartbeat`, exits, and the new Pods crash-loop:
find out what a workload writes *before* making its filesystem read-only.
