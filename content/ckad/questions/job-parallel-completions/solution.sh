set -e
cat <<'YAML' | kubectl apply -f -
apiVersion: batch/v1
kind: Job
metadata: {name: thumbnailer, namespace: app-bobcat}
spec:
  completions: 6
  parallelism: 3
  backoffLimit: 2
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: render
          image: busybox:1.36
          command: ["sh", "-c", "sleep 2; echo \"rendered by $HOSTNAME\""]
YAML
kubectl -n app-bobcat wait --for=condition=complete job/thumbnailer --timeout=180s
kubectl -n app-bobcat get pods -l job-name=thumbnailer -o custom-columns=:metadata.name --no-headers > /opt/course/bobcat/pods
