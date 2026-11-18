kubectl delete namespace app-jackal --ignore-not-found --wait >/dev/null
kubectl create namespace app-jackal >/dev/null
rm -rf /opt/course/jackal
mkdir -p /opt/course/jackal/base
cd /opt/course/jackal/base
cat > deployment.yaml <<'YAML'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: reporter
spec:
  replicas: 1
  selector:
    matchLabels: {app: reporter}
  template:
    metadata:
      labels: {app: reporter}
    spec:
      containers:
        - name: reporter
          image: nginx:1.26
          envFrom:
            - configMapRef:
                name: reporter-config
          ports:
            - containerPort: 80
YAML
cat > service.yaml <<'YAML'
apiVersion: v1
kind: Service
metadata:
  name: reporter
spec:
  selector: {app: reporter}
  ports:
    - port: 80
      targetPort: 80
YAML
cat > kustomization.yaml <<'YAML'
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - deployment.yaml
  - service.yaml
YAML
chown -R candidate:candidate /opt/course/jackal
