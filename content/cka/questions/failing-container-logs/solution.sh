set -e
cd /opt/course/magpie
echo transform > container
kubectl -n team-magpie get pod batch-worker -o jsonpath='{.status.containerStatuses[?(@.name=="transform")].lastState.terminated.exitCode}' > exit-code
# Which command shows the crashed run depends on the moment: `logs` while the
# container waits in CrashLoopBackOff, `logs --previous` while it runs again.
for i in $(seq 1 30); do
  line=$( (kubectl -n team-magpie logs batch-worker -c transform 2>/dev/null; kubectl -n team-magpie logs batch-worker -c transform --previous 2>/dev/null) | grep FATAL | tail -1)
  [ -n "$line" ] && break
  sleep 2
done
[ -n "$line" ] || { echo "no FATAL line in the logs of container transform" >&2; exit 1; }
echo "$line" > reason
