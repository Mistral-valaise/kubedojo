kubectl delete namespace app-marmot --ignore-not-found --wait >/dev/null
kubectl create namespace app-marmot >/dev/null
d=/opt/course/marmot
rm -rf $d && mkdir -p $d
cat > $d/deployment.yaml <<'YAML'
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  name: reports
  namespace: app-marmot
spec:
  replicas: 2
  template:
    metadata:
      labels:
        app: reports
    spec:
      containers:
        - name: web
          image: nginx:1.27-alpine
          ports:
            - containerPort: 80
          resources:
            requests:
              cpu: 50m
              memory: 32Mi
YAML
cat > $d/cronjob.yaml <<'YAML'
apiVersion: batch/v1beta1
kind: CronJob
metadata:
  name: report-export
  namespace: app-marmot
spec:
  schedule: "30 2 * * *"
  concurrencyPolicy: Forbid
  jobTemplate:
    spec:
      template:
        spec:
          restartPolicy: OnFailure
          containers:
            - name: export
              image: busybox:1.36
              command: ["sh", "-c", "echo exporting reports; sleep 5"]
YAML
cat > $d/hpa.yaml <<'YAML'
apiVersion: autoscaling/v2beta1
kind: HorizontalPodAutoscaler
metadata:
  name: reports
  namespace: app-marmot
spec:
  scaleTargetRef:
    apiVersion: extensions/v1beta1
    kind: Deployment
    name: reports
  minReplicas: 2
  maxReplicas: 6
  metrics:
    - type: Resource
      resource:
        name: cpu
        targetAverageUtilization: 70
YAML
chown -R candidate:candidate $d
