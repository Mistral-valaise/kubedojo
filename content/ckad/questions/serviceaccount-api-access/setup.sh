kubectl delete namespace app-seal --ignore-not-found --wait >/dev/null
kubectl create namespace app-seal >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: batch/v1
kind: Job
metadata: {name: nightly-export, namespace: app-seal}
spec:
  backoffLimit: 1
  template:
    spec:
      restartPolicy: Never
      terminationGracePeriodSeconds: 1
      containers:
        - name: export
          image: busybox:1.36
          command: ["sh", "-c", "echo exported 42 rows"]
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: job-watcher, namespace: app-seal}
spec:
  replicas: 2
  selector: {matchLabels: {app: job-watcher}}
  template:
    metadata: {labels: {app: job-watcher}}
    spec:
      terminationGracePeriodSeconds: 1
      containers:
        - name: watcher
          # nginx:1.27-alpine for its curl: busybox wget cannot do HTTPS on amd64.
          image: nginx:1.27-alpine
          command:
            - sh
            - -c
            - |
              sa=/var/run/secrets/kubernetes.io/serviceaccount
              url="https://$KUBERNETES_SERVICE_HOST:$KUBERNETES_SERVICE_PORT/apis/batch/v1/namespaces/$(cat $sa/namespace)/jobs"
              while true; do
                if curl -sfk -m 5 -o /dev/null -H "Authorization: Bearer $(cat $sa/token)" "$url"; then
                  echo "jobs: access ok"
                else
                  echo "jobs: access denied"
                fi
                sleep 5
              done
YAML
kubectl -n app-seal rollout status deployment job-watcher --timeout=180s >/dev/null 2>&1 || true
