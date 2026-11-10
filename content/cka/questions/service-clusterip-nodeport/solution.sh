set -e
kubectl -n team-gull expose deployment catalog --name catalog --port 8080 --target-port 80
cat <<'YAML' | kubectl apply -f -
apiVersion: v1
kind: Service
metadata: {name: catalog-external, namespace: team-gull}
spec:
  type: NodePort
  selector: {app: catalog}
  ports:
    - {port: 80, targetPort: 80, nodePort: 30080}
YAML
