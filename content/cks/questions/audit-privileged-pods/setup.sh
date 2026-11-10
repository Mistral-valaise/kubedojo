kubectl delete namespace sec-wader --ignore-not-found --wait >/dev/null
kubectl create namespace sec-wader >/dev/null
mkdir -p /opt/course/audit && rm -f /opt/course/audit/dangerous-pods && chown -R candidate:candidate /opt/course/audit
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: v1
kind: Pod
metadata: {name: api, namespace: sec-wader}
spec:
  terminationGracePeriodSeconds: 1
  containers:
    - name: api
      image: busybox:1.36
      command: ["sleep", "1d"]
      securityContext: {allowPrivilegeEscalation: false, privileged: false}
---
apiVersion: v1
kind: Pod
metadata: {name: cache-warmer, namespace: sec-wader}
spec:
  terminationGracePeriodSeconds: 1
  volumes: [{name: scratch, emptyDir: {}}]
  containers:
    - name: warmer
      image: busybox:1.36
      command: ["sleep", "1d"]
      volumeMounts: [{name: scratch, mountPath: /scratch}]
---
apiVersion: v1
kind: Pod
metadata: {name: debug-shell, namespace: sec-wader}
spec:
  terminationGracePeriodSeconds: 1
  containers:
    - name: helper
      image: busybox:1.36
      command: ["sleep", "1d"]
    - name: shell
      image: busybox:1.36
      command: ["sleep", "1d"]
      securityContext: {privileged: true}
---
apiVersion: v1
kind: Pod
metadata: {name: log-shipper, namespace: sec-wader}
spec:
  terminationGracePeriodSeconds: 1
  volumes: [{name: logs, hostPath: {path: /var/log}}]
  containers:
    - name: shipper
      image: busybox:1.36
      command: ["sleep", "1d"]
      volumeMounts: [{name: logs, mountPath: /host-logs, readOnly: true}]
---
apiVersion: v1
kind: Pod
metadata: {name: net-tools, namespace: sec-wader}
spec:
  terminationGracePeriodSeconds: 1
  hostNetwork: true
  containers:
    - name: tools
      image: busybox:1.36
      command: ["sleep", "1d"]
YAML
kubectl -n sec-wader wait --for=condition=Ready pod --all --timeout=180s >/dev/null 2>&1 || true
