set -e
mkdir -p /etc/kubernetes/enc /root/.kd
cat > /etc/kubernetes/enc/enc.yaml <<YAML
apiVersion: apiserver.config.k8s.io/v1
kind: EncryptionConfiguration
resources:
  - resources:
      - secrets
    providers:
      - aescbc:
          keys:
            - name: key1
              secret: $(head -c 32 /dev/urandom | base64)
      - identity: {}
YAML
f=/etc/kubernetes/manifests/kube-apiserver.yaml
awk '
/^    - kube-apiserver$/ {print; print "    - --encryption-provider-config=/etc/kubernetes/enc/enc.yaml"; next}
/^    volumeMounts:$/ {print; print "    - mountPath: /etc/kubernetes/enc"; print "      name: enc"; print "      readOnly: true"; next}
/^  volumes:$/ {print; print "  - hostPath:"; print "      path: /etc/kubernetes/enc"; print "      type: DirectoryOrCreate"; print "    name: enc"; next}
{print}' $f > /root/.kd/api.tmp
mv /root/.kd/api.tmp $f
for i in $(seq 1 90); do
  pgrep -a kube-apiserver | grep -q -- '--encryption-provider-config=' && kubectl get --raw /readyz >/dev/null 2>&1 && break; sleep 2
done
kubectl -n sec-pelican get secret db-pass -o json | kubectl replace -f -
