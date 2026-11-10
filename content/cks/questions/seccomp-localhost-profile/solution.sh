set -e
ssh ${KD_HOST}-worker 'mkdir -p /var/lib/kubelet/seccomp/profiles'
scp -q /opt/course/seccomp/deny-dangerous.json ${KD_HOST}-worker:/var/lib/kubelet/seccomp/profiles/deny-dangerous.json
cat <<'YAML' | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata: {name: hardened, namespace: sec-lark}
spec:
  securityContext:
    seccompProfile:
      type: Localhost
      localhostProfile: profiles/deny-dangerous.json
  containers:
    - {name: nginx, image: "nginx:1.27-alpine"}
YAML
