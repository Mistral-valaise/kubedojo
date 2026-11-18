set -e
kubectl -n app-koala patch deployment shop --type strategic -p '
spec:
  template:
    spec:
      containers:
        - name: web
          readinessProbe:
            httpGet: {path: /ready, port: 8080}
            periodSeconds: 5
          livenessProbe:
            tcpSocket: {port: 8080}
            initialDelaySeconds: 10
            periodSeconds: 10
'
kubectl -n app-koala rollout status deployment shop --timeout=180s
