# expect: pass
# Regression test for issue #1: a first attempt logs everything, then the
# policy is corrected. The log still holds hundreds of lines from the first
# policy; they must not count against the corrected one.
set -e
mkdir -p /etc/kubernetes/audit /var/log/kubernetes/audit /root/.kd
cat > /etc/kubernetes/audit/policy.yaml <<'YAML'
apiVersion: audit.k8s.io/v1
kind: Policy
rules:
  - level: Metadata
YAML
f=/etc/kubernetes/manifests/kube-apiserver.yaml
if ! grep -q -- '--audit-policy-file=' $f; then
awk '
/^    - kube-apiserver$/ {print; print "    - --audit-policy-file=/etc/kubernetes/audit/policy.yaml"; print "    - --audit-log-path=/var/log/kubernetes/audit/audit.log"; print "    - --audit-log-maxage=7"; print "    - --audit-log-maxbackup=2"; next}
/^    volumeMounts:$/ {print; print "    - mountPath: /etc/kubernetes/audit/policy.yaml"; print "      name: audit-policy"; print "      readOnly: true"; print "    - mountPath: /var/log/kubernetes/audit"; print "      name: audit-log"; next}
/^  volumes:$/ {print; print "  - hostPath:"; print "      path: /etc/kubernetes/audit/policy.yaml"; print "      type: File"; print "    name: audit-policy"; print "  - hostPath:"; print "      path: /var/log/kubernetes/audit"; print "      type: DirectoryOrCreate"; print "    name: audit-log"; next}
{print}' $f > /root/.kd/api.tmp
mv /root/.kd/api.tmp $f
fi
for i in $(seq 1 90); do
  pgrep -a kube-apiserver | grep -q -- '--audit-policy-file=' && kubectl get --raw /readyz >/dev/null 2>&1 && break; sleep 2
done
sleep 20
cat > /etc/kubernetes/audit/policy.yaml <<'YAML'
apiVersion: audit.k8s.io/v1
kind: Policy
rules:
  - level: RequestResponse
    resources:
      - group: ""
        resources: ["pods"]
  - level: Metadata
    resources:
      - group: ""
        resources: ["secrets", "configmaps"]
  - level: None
YAML
# The API server reads the policy only when it starts: restart the static Pod.
old=$(pgrep kube-apiserver)
mv $f /root/.kd/kube-apiserver.yaml; sleep 8; mv /root/.kd/kube-apiserver.yaml $f
for i in $(seq 1 90); do
  p=$(pgrep kube-apiserver || true); [ -n "$p" ] && [ "$p" != "$old" ] && kubectl get --raw /readyz >/dev/null 2>&1 && break; sleep 2
done
