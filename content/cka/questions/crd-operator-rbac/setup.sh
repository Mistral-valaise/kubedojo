kubectl delete namespace team-egret --ignore-not-found --wait >/dev/null
kubectl delete clusterrole backup-operator --ignore-not-found >/dev/null
kubectl delete clusterrolebinding backup-operator --ignore-not-found >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: backups.ops.kubedojo.dev
spec:
  group: ops.kubedojo.dev
  scope: Namespaced
  names: {plural: backups, singular: backup, kind: Backup, shortNames: [bkp]}
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              required: [schedule]
              properties:
                schedule: {type: string, description: "Cron expression that defines when the backup runs."}
                retentionDays: {type: integer, minimum: 1, description: "Number of days a backup is kept."}
      additionalPrinterColumns:
        - {name: Schedule, type: string, jsonPath: .spec.schedule}
        - {name: Retention, type: integer, jsonPath: .spec.retentionDays}
YAML
kubectl wait --for=condition=Established crd/backups.ops.kubedojo.dev --timeout=60s >/dev/null
kubectl create namespace team-egret >/dev/null
kubectl -n team-egret create serviceaccount backup-operator >/dev/null
