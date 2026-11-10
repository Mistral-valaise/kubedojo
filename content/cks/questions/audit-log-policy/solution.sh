set -e
mkdir -p /etc/kubernetes/audit /var/log/kubernetes/audit /root/.kd
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
f=/etc/kubernetes/manifests/kube-apiserver.yaml
awk '
/^    - kube-apiserver$/ {print; print "    - --audit-policy-file=/etc/kubernetes/audit/policy.yaml"; print "    - --audit-log-path=/var/log/kubernetes/audit/audit.log"; print "    - --audit-log-maxage=7"; print "    - --audit-log-maxbackup=2"; next}
/^    volumeMounts:$/ {print; print "    - mountPath: /etc/kubernetes/audit/policy.yaml"; print "      name: audit-policy"; print "      readOnly: true"; print "    - mountPath: /var/log/kubernetes/audit"; print "      name: audit-log"; next}
/^  volumes:$/ {print; print "  - hostPath:"; print "      path: /etc/kubernetes/audit/policy.yaml"; print "      type: File"; print "    name: audit-policy"; print "  - hostPath:"; print "      path: /var/log/kubernetes/audit"; print "      type: DirectoryOrCreate"; print "    name: audit-log"; next}
{print}' $f > /root/.kd/api.tmp
mv /root/.kd/api.tmp $f
