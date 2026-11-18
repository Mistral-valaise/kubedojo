kubectl delete namespace app-hare --ignore-not-found --wait >/dev/null
kubectl create namespace app-hare >/dev/null
rm -rf /opt/course/hare
c=/opt/course/hare/ledger
mkdir -p $c/templates
cat > $c/Chart.yaml <<'YAML'
apiVersion: v2
name: ledger
description: Bookkeeping API of team Hare
version: 1.4.0
appVersion: "1.27"
YAML
cat > $c/values.yaml <<'YAML'
replicaCount: 2
image:
  repository: nginx
  tag: 1.27-alpine
config:
  logLevel: info
service:
  port: 80
YAML
cat > $c/templates/deployment.yaml <<'YAML'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}
  labels: {app: {{ .Release.Name }}}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels: {app: {{ .Release.Name }}}
  template:
    metadata:
      labels: {app: {{ .Release.Name }}}
    spec:
      terminationGracePeriodSeconds: 2
      containers:
        - name: api
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          env:
            - name: LOG_LEVEL
              value: {{ .Values.config.logLevel | quote }}
          ports: [{containerPort: 80}]
          readinessProbe:
            httpGet: {path: /, port: 80}
            periodSeconds: 2
YAML
cat > $c/templates/service.yaml <<'YAML'
apiVersion: v1
kind: Service
metadata:
  name: {{ .Release.Name }}
spec:
  selector: {app: {{ .Release.Name }}}
  ports:
    - port: {{ .Values.service.port }}
      targetPort: 80
YAML
chown -R candidate:candidate /opt/course/hare

# Revision 1: defaults. Revision 2: scaled to 3. Revision 3: an image tag that does not exist.
helm -n app-hare install ledger $c >/dev/null
kubectl -n app-hare rollout status deploy/ledger --timeout=180s >/dev/null 2>&1 || true
helm -n app-hare upgrade ledger $c --reuse-values --set replicaCount=3 >/dev/null
kubectl -n app-hare rollout status deploy/ledger --timeout=180s >/dev/null 2>&1 || true
helm -n app-hare upgrade ledger $c --reuse-values --set image.tag=1.72-alpine >/dev/null
# The learner should find the rollout stuck, not a cluster that still looks healthy.
for i in $(seq 60); do
  kubectl -n app-hare get pods -o jsonpath='{.items[*].status.containerStatuses[*].state.waiting.reason}' 2>/dev/null \
    | grep -Eq 'ErrImagePull|ImagePullBackOff' && break
  sleep 2
done
