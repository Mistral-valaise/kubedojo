f=/etc/kubernetes/manifests/kube-apiserver.yaml
mkdir -p /root/.kd
sed -e 's/--enable-admission-plugins=[^ ]*/--enable-admission-plugins=NamespaceLifecycle/' -e '/--profiling=/d' $f > /root/.kd/api.tmp
sed -i 's/^    - --enable-admission-plugins=NamespaceLifecycle$/&\n    - --profiling=true/' /root/.kd/api.tmp
mv /root/.kd/api.tmp $f
for i in $(seq 1 90); do
  pgrep -a kube-apiserver | grep -q -- '--profiling=true' && kubectl get --raw /readyz >/dev/null 2>&1 && break; sleep 2
done
exit 0
