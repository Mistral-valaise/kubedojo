kubectl delete namespace team-magpie --ignore-not-found --wait >/dev/null
kubectl create namespace team-magpie >/dev/null
mkdir -p /opt/course/magpie && rm -f /opt/course/magpie/* && chown -R candidate:candidate /opt/course/magpie
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: v1
kind: Pod
metadata:
  name: batch-worker
  namespace: team-magpie
spec:
  terminationGracePeriodSeconds: 1
  containers:
    - name: fetch
      image: busybox:1.36
      command: ["sh", "-c", "while true; do echo fetched batch $(date +%s); sleep 15; done"]
    - name: transform
      image: busybox:1.36
      command: ["sh", "-c", "echo loading schema; sleep 2; echo 'FATAL: unsupported schema version 7'; exit 42"]
    - name: ship
      image: busybox:1.36
      command: ["sh", "-c", "while true; do echo waiting for transformed data; sleep 15; done"]
YAML
for i in $(seq 1 60); do
  [ -n "$(kubectl -n team-magpie get pod batch-worker -o jsonpath='{.status.containerStatuses[?(@.name=="transform")].lastState.terminated.exitCode}' 2>/dev/null)" ] && break; sleep 2
done
