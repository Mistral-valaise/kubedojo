kubectl delete namespace sec-tern --ignore-not-found --wait >/dev/null
kubectl create namespace sec-tern >/dev/null
mkdir -p /opt/course/forensics && rm -f /opt/course/forensics/deployment && chown -R candidate:candidate /opt/course/forensics
mk() { kubectl -n sec-tern create configmap $1-entrypoint --from-literal=start.sh="$2" >/dev/null
cat <<YAML | kubectl apply -f - >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata: {name: $1, namespace: sec-tern}
spec:
  replicas: 1
  selector: {matchLabels: {app: $1}}
  template:
    metadata: {labels: {app: $1}}
    spec:
      terminationGracePeriodSeconds: 1
      volumes:
        - name: entrypoint
          configMap: {name: $1-entrypoint, defaultMode: 493}
      containers:
        - name: app
          image: busybox:1.36
          command: ["/entrypoint/start.sh"]
          volumeMounts: [{name: entrypoint, mountPath: /entrypoint}]
YAML
}
mk frontend '#!/bin/sh
while true; do echo serving; sleep 30; done'
mk order-worker '#!/bin/sh
while true; do echo processing orders; sleep 30; done'
mk metrics-relay '#!/bin/sh
printf "#!/bin/sh\nwhile true; do sleep 30; done\n" > /tmp/kworkerd; chmod +x /tmp/kworkerd
/tmp/kworkerd &
while true; do echo relaying metrics; sleep 30; done'
kubectl -n sec-tern rollout status deploy/frontend deploy/order-worker deploy/metrics-relay --timeout=180s >/dev/null 2>&1 || true
