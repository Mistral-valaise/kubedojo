set -e
cat <<'YAML' | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata: {name: cp-probe, namespace: team-wader}
spec:
  terminationGracePeriodSeconds: 1
  nodeSelector:
    node-role.kubernetes.io/control-plane: ""
  tolerations:
    - {key: node-role.kubernetes.io/control-plane, effect: NoSchedule}
  containers:
    - {name: probe, image: "busybox:1.36", command: ["sleep", "1d"]}
YAML
