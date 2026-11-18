kubectl delete namespace app-moose --ignore-not-found --wait >/dev/null
kubectl create namespace app-moose >/dev/null
rm -rf /opt/course/moose && mkdir -p /opt/course/moose && chown -R candidate:candidate /opt/course/moose
cat <<'YAML' | kubectl apply -f - >/dev/null
apiVersion: v1
kind: ConfigMap
metadata:
  name: orders-config
  namespace: app-moose
data:
  LOG_LEVEL: info
  app.conf: |
    # orders-api settings
    PORT=8080
    GREETING="orders-api ok"
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: orders-scripts
  namespace: app-moose
data:
  start.sh: |
    #!/bin/sh
    echo "orders-api starting, log level ${LOG_LEVEL}"
    if [ ! -f "${CONFIG_FILE}" ]; then
      echo "FATAL: config file ${CONFIG_FILE} not found"
      exit 3
    fi
    . "${CONFIG_FILE}"
    mkdir -p /www && echo "${GREETING}" > /www/index.html
    echo "orders-api listening on :${PORT}"
    exec httpd -f -p "${PORT}" -h /www
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: orders
  namespace: app-moose
spec:
  replicas: 2
  selector:
    matchLabels: {app: orders}
  template:
    metadata:
      labels: {app: orders}
    spec:
      terminationGracePeriodSeconds: 2
      containers:
        - name: api
          image: busybox:1.36
          command: ["/bin/sh", "/opt/app/start.sh"]
          env:
            - name: CONFIG_FILE
              value: /etc/orders/orders.conf
            - name: LOG_LEVEL
              valueFrom:
                configMapKeyRef:
                  name: orders-config
                  key: log_level
          ports:
            - name: http
              containerPort: 80
          volumeMounts:
            - {name: config, mountPath: /etc/orders}
            - {name: scripts, mountPath: /opt/app}
      volumes:
        - name: config
          configMap:
            name: orders-config
            items:
              - {key: app.conf, path: app.conf}
        - name: scripts
          configMap: {name: orders-scripts}
---
apiVersion: v1
kind: Service
metadata:
  name: orders
  namespace: app-moose
spec:
  selector: {app: orders}
  ports:
    - port: 80
      targetPort: http
YAML
# Wait until the first fault is visible.
for i in $(seq 1 60); do
  kubectl -n app-moose get pod -l app=orders -o jsonpath='{.items[*].status.containerStatuses[*].state.waiting.reason}' 2>/dev/null | grep -q CreateContainerConfigError && break
  sleep 2
done
