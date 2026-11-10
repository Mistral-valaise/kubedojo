rm -f /etc/kubernetes/manifests/sentinel.yaml /etc/kubernetes/manifests/sentinel.yml
for i in $(seq 1 30); do kubectl -n default get pod sentinel-${KD_HOST}-control-plane >/dev/null 2>&1 || break; sleep 2; done
exit 0
