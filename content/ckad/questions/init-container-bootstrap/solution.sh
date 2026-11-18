set -e
kubectl -n app-cougar patch deploy docs --type strategic -p '
spec:
  template:
    spec:
      volumes:
        - {name: site, emptyDir: {}}
      initContainers:
        - name: fetch-content
          image: busybox:1.36
          command: ["sh", "-c", "echo \"cougar docs build 7 on $(hostname)\" > /site/index.html"]
          volumeMounts: [{name: site, mountPath: /site}]
      containers:
        - name: web
          volumeMounts: [{name: site, mountPath: /usr/share/nginx/html}]
'
kubectl -n app-cougar rollout status deploy/docs --timeout=180s
