kubectl delete namespace team-otter --ignore-not-found --wait >/dev/null
kubectl create namespace team-otter >/dev/null
c=/opt/course/helm/webcache
mkdir -p $c/templates
cat > $c/Chart.yaml <<'YAML'
apiVersion: v2
name: webcache
description: A small caching web tier
version: 0.3.1
appVersion: "1.27"
YAML
cat > $c/values.yaml <<'YAML'
replicaCount: 1
image:
  repository: nginx
  tag: 1.27-alpine
service:
  type: ClusterIP
  port: 80
YAML
cat > $c/templates/deployment.yaml <<'YAML'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}-webcache
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels: {app: {{ .Release.Name }}-webcache}
  template:
    metadata:
      labels: {app: {{ .Release.Name }}-webcache}
    spec:
      containers:
        - name: web
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          ports: [{containerPort: 80}]
YAML
cat > $c/templates/service.yaml <<'YAML'
apiVersion: v1
kind: Service
metadata:
  name: {{ .Release.Name }}-webcache
spec:
  type: {{ .Values.service.type }}
  selector: {app: {{ .Release.Name }}-webcache}
  ports:
    - port: {{ .Values.service.port }}
      targetPort: 80
YAML
chown -R candidate:candidate /opt/course/helm
helm -n team-otter install old-cache $c >/dev/null
