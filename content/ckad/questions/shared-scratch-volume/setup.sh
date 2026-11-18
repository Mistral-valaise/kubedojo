kubectl delete namespace app-coyote --ignore-not-found --wait >/dev/null
kubectl create namespace app-coyote >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata: {name: ledger, namespace: app-coyote, labels: {app: ledger}}
spec:
  replicas: 1
  selector: {matchLabels: {app: ledger}}
  template:
    metadata: {labels: {app: ledger}}
    spec:
      terminationGracePeriodSeconds: 2
      containers:
        - name: ledger
          image: busybox:1.36
          command:
            - sh
            - -c
            - |
              while true; do
                date > /var/cache/ledger/heartbeat || exit 1
                echo "entry $(date +%s)" >> /var/lib/ledger/journal.log || exit 1
                sleep 2
              done
          readinessProbe:
            exec: {command: ["test", "-s", "/var/lib/ledger/journal.log"]}
            periodSeconds: 2
          securityContext:
            runAsUser: 10001
            runAsNonRoot: true
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
YAML
# Let the learner find the Pod already failing.
for _ in $(seq 1 30); do
  s=$(kubectl -n app-coyote get pod -l app=ledger -o jsonpath='{.items[0].status.containerStatuses[0].restartCount}{.items[0].status.containerStatuses[0].state.terminated.reason}' 2>/dev/null)
  case "$s" in ""|0) sleep 2 ;; *) break ;; esac
done
