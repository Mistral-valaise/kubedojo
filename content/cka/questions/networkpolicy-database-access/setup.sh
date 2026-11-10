kubectl delete namespace team-ibex team-ibex-monitoring --ignore-not-found --wait >/dev/null
kubectl create namespace team-ibex >/dev/null
kubectl create namespace team-ibex-monitoring >/dev/null
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata: {name: db, namespace: team-ibex}
spec:
  replicas: 1
  selector: {matchLabels: {app: db}}
  template:
    metadata: {labels: {app: db}}
    spec:
      terminationGracePeriodSeconds: 1
      containers:
        - name: db
          image: busybox:1.36
          command: ["sh", "-c", "while true; do nc -l -p 5432 -e /bin/true; done"]
---
apiVersion: v1
kind: Service
metadata: {name: db, namespace: team-ibex}
spec:
  selector: {app: db}
  ports: [{port: 5432}]
YAML
mk() { cat <<YAML | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata: {name: $2, namespace: $1}
spec:
  replicas: 1
  selector: {matchLabels: {app: $2}}
  template:
    metadata: {labels: {app: $2}}
    spec:
      terminationGracePeriodSeconds: 1
      containers:
        - {name: app, image: "busybox:1.36", command: ["sleep", "1d"]}
YAML
}
mk team-ibex api; mk team-ibex web; mk team-ibex-monitoring probe
kubectl -n team-ibex rollout status deploy/db deploy/api deploy/web --timeout=180s >/dev/null 2>&1 || true
kubectl -n team-ibex-monitoring rollout status deploy/probe --timeout=180s >/dev/null 2>&1 || true
