set -e
kubectl -n kube-system get cm coredns -o jsonpath='{.data.Corefile}' > /opt/course/dns/Corefile.bak
awk '/^[ \t]*kubernetes cluster.local/ && !done {print "    hosts {\n        10.20.30.40 legacy-db.corp.internal\n        fallthrough\n    }"; done=1} {print}' /opt/course/dns/Corefile.bak > /root/.kd/Corefile.new
kubectl -n kube-system create configmap coredns --from-file=Corefile=/root/.kd/Corefile.new --dry-run=client -o yaml | kubectl apply -f -
