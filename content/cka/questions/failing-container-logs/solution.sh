set -e
cd /opt/course/magpie
echo transform > container
kubectl -n team-magpie get pod batch-worker -o jsonpath='{.status.containerStatuses[?(@.name=="transform")].lastState.terminated.exitCode}' > exit-code
(kubectl -n team-magpie logs batch-worker -c transform --previous 2>/dev/null; sleep 3; kubectl -n team-magpie logs batch-worker -c transform 2>/dev/null) | grep FATAL | tail -1 > reason
