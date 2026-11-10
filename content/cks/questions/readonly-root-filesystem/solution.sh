set -e
kubectl -n sec-osprey patch deployment cache --type strategic -p '
spec:
  template:
    spec:
      volumes:
        - {name: cache, emptyDir: {}}
        - {name: run, emptyDir: {}}
      containers:
        - name: nginx
          securityContext:
            readOnlyRootFilesystem: true
            allowPrivilegeEscalation: false
          volumeMounts:
            - {name: cache, mountPath: /var/cache/nginx}
            - {name: run, mountPath: /var/run}
'
kubectl -n sec-osprey rollout status deployment cache --timeout=120s
