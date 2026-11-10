set -e
kubeadm token create --ttl 2h --print-join-command > /opt/course/join/join.sh
