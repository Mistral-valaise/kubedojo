kubectl delete namespace team-lark --ignore-not-found --wait >/dev/null
kubectl create namespace team-lark >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata: {name: app, namespace: team-lark}
spec:
  replicas: 1
  selector: {matchLabels: {app: app}}
  template:
    metadata: {labels: {app: app}}
    spec:
      terminationGracePeriodSeconds: 1
      containers:
        - name: app
          image: busybox:1.36
          command: ["sleep", "1d"]
YAML
