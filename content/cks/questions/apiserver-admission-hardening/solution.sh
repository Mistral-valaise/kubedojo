set -e
f=/etc/kubernetes/manifests/kube-apiserver.yaml
sed -e 's/--enable-admission-plugins=[^ ]*/--enable-admission-plugins=NodeRestriction/' -e 's/--profiling=true/--profiling=false/' $f > /root/.kd/api.tmp
mv /root/.kd/api.tmp $f
