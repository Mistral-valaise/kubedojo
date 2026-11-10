set -e
pid=$(ssh ${KD_HOST}-worker "pgrep -f '^/bin/sh /tmp/kworkerd' | head -1")
cid=$(ssh ${KD_HOST}-worker "grep -o 'cri-containerd-[a-f0-9]*' /proc/$pid/cgroup | head -1 | sed 's/cri-containerd-//'")
pod=$(ssh ${KD_HOST}-worker "crictl inspect $cid" | jq -r '.status.labels["io.kubernetes.pod.name"]')
dep=$(kubectl -n sec-tern get pod "$pod" -o jsonpath='{.metadata.labels.app}')
echo "$dep" > /opt/course/forensics/deployment
kubectl -n sec-tern scale deployment "$dep" --replicas=0
kubectl -n sec-tern wait --for=delete pod -l app="$dep" --timeout=60s
