kubectl delete namespace sec-dunlin --ignore-not-found --wait >/dev/null
kubectl create namespace sec-dunlin >/dev/null
mkdir -p /opt/course/drift /root/.kd && rm -f /opt/course/drift/pod /root/.kd/drift-pod && chown -R candidate:candidate /opt/course/drift
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata: {name: ledger, namespace: sec-dunlin}
spec:
  replicas: 3
  selector: {matchLabels: {app: ledger}}
  template:
    metadata: {labels: {app: ledger}}
    spec:
      terminationGracePeriodSeconds: 1
      containers:
        - name: ledger
          image: busybox:1.36
          command: ["sh", "-c", "mkdir -p /data && while true; do date > /data/heartbeat || exit 1; sleep 5; done"]
YAML
kubectl -n sec-dunlin rollout status deployment ledger --timeout=180s >/dev/null 2>&1 || true
pod=$(kubectl -n sec-dunlin get pod -l app=ledger --sort-by=.metadata.name -o jsonpath='{.items[1].metadata.name}')
[ -n "$pod" ] || { echo "no ledger Pod to modify" >&2; exit 1; }
kubectl -n sec-dunlin exec "$pod" -- sh -c 'printf "#!/bin/sh\nwhile true; do sleep 3600; done\n" > /bin/kthrotld && chmod 755 /bin/kthrotld && (nohup /bin/kthrotld >/dev/null 2>&1 &) ; sleep 1' >/dev/null
kubectl -n sec-dunlin exec "$pod" -- sh -c 'ps | grep -q "[k]throtld"' || { echo "could not start the extra process" >&2; exit 1; }
echo "$pod" > /root/.kd/drift-pod
chmod 600 /root/.kd/drift-pod
