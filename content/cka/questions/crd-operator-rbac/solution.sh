set -e
kubectl create clusterrole backup-operator --verb=get,list,watch,update --resource=backups.ops.kubedojo.dev
kubectl -n team-egret create rolebinding backup-operator --clusterrole=backup-operator --serviceaccount=team-egret:backup-operator
cat <<'YAML' | kubectl apply -f -
apiVersion: ops.kubedojo.dev/v1
kind: Backup
metadata: {name: nightly, namespace: team-egret}
spec:
  schedule: "0 2 * * *"
  retentionDays: 14
YAML
