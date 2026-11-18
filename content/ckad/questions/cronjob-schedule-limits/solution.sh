set -e
cat <<'YAML' | kubectl apply -f -
apiVersion: batch/v1
kind: CronJob
metadata: {name: report-sync, namespace: app-beaver}
spec:
  schedule: "*/15 * * * *"
  concurrencyPolicy: Forbid
  startingDeadlineSeconds: 45
  successfulJobsHistoryLimit: 2
  failedJobsHistoryLimit: 4
  jobTemplate:
    spec:
      activeDeadlineSeconds: 30
      template:
        spec:
          restartPolicy: OnFailure
          containers:
            - name: sync
              image: busybox:1.36
              command: ["sh", "-c", "echo report synced"]
YAML
kubectl -n app-beaver create job report-sync-manual --from=cronjob/report-sync
kubectl -n app-beaver wait --for=condition=complete job/report-sync-manual --timeout=120s
