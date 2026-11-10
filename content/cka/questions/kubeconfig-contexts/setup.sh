mkdir -p /opt/course/kubeconfig /root/.kd/kc && cd /opt/course/kubeconfig
rm -f /opt/course/kubeconfig/contexts /opt/course/kubeconfig/current /opt/course/kubeconfig/lena.crt /opt/course/kubeconfig/fleet.yaml
for u in ops-root lena viewer; do
  openssl req -x509 -newkey rsa:2048 -nodes -days 365 -subj "/CN=$u/O=fleet" -keyout /root/.kd/kc/$u.key -out /root/.kd/kc/$u.crt >/dev/null 2>&1
done
b() { base64 -w0 "$1"; }
cat > fleet.yaml <<YAML
apiVersion: v1
kind: Config
clusters:
- name: fleet
  cluster:
    server: https://fleet.example.internal:6443
    certificate-authority-data: $(b /etc/kubernetes/pki/ca.crt)
contexts:
- name: fleet-admin
  context: {cluster: fleet, user: ops-root}
- name: fleet-dev
  context: {cluster: fleet, user: lena, namespace: dev}
- name: fleet-readonly
  context: {cluster: fleet, user: viewer}
current-context: fleet-dev
users:
- name: ops-root
  user:
    client-certificate-data: $(b /root/.kd/kc/ops-root.crt)
    client-key-data: $(b /root/.kd/kc/ops-root.key)
- name: viewer
  user:
    client-certificate-data: $(b /root/.kd/kc/viewer.crt)
    client-key-data: $(b /root/.kd/kc/viewer.key)
- name: lena
  user:
    client-certificate-data: $(b /root/.kd/kc/lena.crt)
    client-key-data: $(b /root/.kd/kc/lena.key)
YAML
chown -R candidate:candidate /opt/course/kubeconfig
