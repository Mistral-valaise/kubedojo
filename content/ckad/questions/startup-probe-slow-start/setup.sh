kubectl delete namespace app-lemur --ignore-not-found --wait >/dev/null
kubectl create namespace app-lemur >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ledger
  namespace: app-lemur
spec:
  replicas: 2
  selector:
    matchLabels: {app: ledger}
  template:
    metadata:
      labels: {app: ledger}
    spec:
      terminationGracePeriodSeconds: 2
      containers:
        - name: ledger
          image: busybox:1.36
          command:
            - sh
            - -c
            - |
              mkdir -p /www && echo ok > /www/healthz
              echo "ledger: replaying journal, this takes a while"
              sleep 25
              echo "ledger: journal replayed, listening on :8080"
              exec httpd -f -p 8080 -h /www
          ports:
            - name: http
              containerPort: 8080
          livenessProbe:
            httpGet: {path: /healthz, port: http}
            periodSeconds: 5
            failureThreshold: 3
          readinessProbe:
            httpGet: {path: /healthz, port: http}
            periodSeconds: 3
YAML
# Let the learner find the broken state: wait until the liveness probe has killed a container.
for i in $(seq 1 60); do
  n=$(kubectl -n app-lemur get pod -l app=ledger -o jsonpath='{.items[*].status.containerStatuses[*].restartCount}' 2>/dev/null | tr ' ' '\n' | sort -n | tail -1)
  [ "${n:-0}" -ge 1 ] && break
  sleep 2
done
