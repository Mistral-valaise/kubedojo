set -e
cat <<'YAML' | kubectl apply -f -
apiVersion: v1
kind: Service
metadata: {name: kv, namespace: team-robin}
spec:
  clusterIP: None
  selector: {app: kv}
  ports: [{port: 80}]
YAML
echo kv-1.kv.team-robin.svc.cluster.local > /opt/course/robin/fqdn
