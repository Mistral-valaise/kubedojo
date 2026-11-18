set -e
ns=app-moose
# Fault 1: the ConfigMap key is LOG_LEVEL, not log_level (CreateContainerConfigError).
kubectl -n $ns patch deployment orders --type json -p '[
 {"op":"replace","path":"/spec/template/spec/containers/0/env/1/valueFrom/configMapKeyRef/key","value":"LOG_LEVEL"}]'
# Fault 2 shows now: the container starts and exits. Read the log while it crashes.
# `logs` shows the crashed run while the container waits in CrashLoopBackOff, `--previous`
# shows it while the container runs again; which one works depends on the moment, so poll.
line=""
for i in $(seq 1 90); do
  for p in $(kubectl -n $ns get pod -l app=orders -o name); do
    line=$( (kubectl -n $ns logs "$p" 2>/dev/null; kubectl -n $ns logs "$p" --previous 2>/dev/null) | grep '^FATAL' | tail -1)
    [ -n "$line" ] && break 2
  done
  sleep 2
done
[ -n "$line" ] || { echo "no FATAL line in the logs of the orders Pods" >&2; exit 1; }
echo "$line" > /opt/course/moose/root-cause
# Fault 2: the config file is mounted as /etc/orders/app.conf.
# Fault 3: Service targetPort "http" must resolve to the port the app listens on.
kubectl -n $ns patch deployment orders --type json -p '[
 {"op":"replace","path":"/spec/template/spec/containers/0/env/0/value","value":"/etc/orders/app.conf"},
 {"op":"replace","path":"/spec/template/spec/containers/0/ports/0/containerPort","value":8080}]'
kubectl -n $ns rollout status deployment orders --timeout=180s
