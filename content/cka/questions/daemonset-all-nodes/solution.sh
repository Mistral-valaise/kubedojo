set -e
cat <<'YAML' | kubectl apply -f -
apiVersion: apps/v1
kind: DaemonSet
metadata: {name: node-agent, namespace: team-owl}
spec:
  selector: {matchLabels: {app: node-agent}}
  template:
    metadata: {labels: {app: node-agent}}
    spec:
      terminationGracePeriodSeconds: 1
      tolerations:
        - {key: node-role.kubernetes.io/control-plane, effect: NoSchedule}
      containers:
        - name: agent
          image: busybox:1.36
          command: ["sleep", "1d"]
          resources: {requests: {cpu: 10m, memory: 16Mi}}
YAML
