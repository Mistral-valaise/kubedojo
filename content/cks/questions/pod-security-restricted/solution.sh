set -e
kubectl label namespace sec-magpie pod-security.kubernetes.io/enforce=restricted --overwrite
kubectl -n sec-magpie patch deployment web --type strategic -p '
spec:
  template:
    spec:
      containers:
        - name: web
          securityContext:
            allowPrivilegeEscalation: false
            runAsNonRoot: true
            capabilities: {drop: [ALL]}
            seccompProfile: {type: RuntimeDefault}
'
kubectl -n sec-magpie rollout status deployment web --timeout=180s
