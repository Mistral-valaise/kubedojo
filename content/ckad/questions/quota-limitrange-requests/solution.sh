set -e
kubectl -n app-rabbit get events --field-selector reason=FailedCreate
kubectl -n app-rabbit set resources deployment worker --requests=cpu=200m --limits=cpu=400m
kubectl -n app-rabbit rollout status deployment worker --timeout=150s
# the quota controller needs a moment to count the new Pods
for i in $(seq 1 30); do
  [ "$(kubectl -n app-rabbit get resourcequota rabbit-quota -o jsonpath='{.status.used.requests\.cpu}')" = "850m" ] && break; sleep 1
done
kubectl -n app-rabbit get resourcequota rabbit-quota -o jsonpath='{.status.used.requests\.cpu}/{.status.hard.requests\.cpu}{"\n"}' > /opt/course/rabbit/cpu-requests
