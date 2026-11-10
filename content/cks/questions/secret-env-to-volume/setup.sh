kubectl delete namespace sec-quail --ignore-not-found --wait >/dev/null
kubectl create namespace sec-quail >/dev/null
cat <<'YAML' | kubectl create -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata: {name: payments, namespace: sec-quail}
spec:
  replicas: 1
  selector: {matchLabels: {app: payments}}
  template:
    metadata: {labels: {app: payments}}
    spec:
      terminationGracePeriodSeconds: 1
      containers:
        - name: payments
          image: busybox:1.36
          command: ["sleep", "1d"]
          env:
            - {name: DB_HOST, value: db.sec-quail.svc}
            - {name: DB_PASSWORD, value: hunter2-quail}
YAML
