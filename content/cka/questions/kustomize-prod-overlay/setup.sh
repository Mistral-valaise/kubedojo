kubectl delete namespace team-swift --ignore-not-found --wait >/dev/null
kubectl create namespace team-swift >/dev/null
mkdir -p /opt/course/kustomize/base
rm -rf /opt/course/kustomize/overlays
cd /opt/course/kustomize/base
cat > deployment.yaml <<'YAML'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: notifier
spec:
  replicas: 1
  selector:
    matchLabels: {app: notifier}
  template:
    metadata:
      labels: {app: notifier}
    spec:
      containers:
        - name: notifier
          image: nginx:1.27-alpine
YAML
cat > service.yaml <<'YAML'
apiVersion: v1
kind: Service
metadata:
  name: notifier
spec:
  selector: {app: notifier}
  ports: [{port: 80}]
YAML
cat > kustomization.yaml <<'YAML'
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - deployment.yaml
  - service.yaml
YAML
chown -R candidate:candidate /opt/course/kustomize
