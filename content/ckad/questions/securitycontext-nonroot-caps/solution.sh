set -e
kubectl -n app-puma patch deployment edge --type strategic -p '
spec:
  template:
    spec:
      containers:
        - name: httpd
          securityContext:
            runAsUser: 10001
            runAsGroup: 10001
            runAsNonRoot: true
            allowPrivilegeEscalation: false
            capabilities:
              drop: [ALL]
              add: [NET_BIND_SERVICE]
'
kubectl -n app-puma rollout status deployment edge --timeout=120s
