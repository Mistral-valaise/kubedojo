set -e
cat > /root/sentinel.yaml <<'YAML'
apiVersion: v1
kind: Pod
metadata:
  name: sentinel
  namespace: default
  labels:
    role: sentinel
spec:
  containers:
    - name: sentinel
      image: nginx:1.27-alpine
      resources:
        requests:
          cpu: 10m
          memory: 20Mi
YAML
mv /root/sentinel.yaml /etc/kubernetes/manifests/sentinel.yaml
