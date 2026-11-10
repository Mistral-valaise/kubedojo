set -e
openssl x509 -in /etc/kubernetes/pki/apiserver.crt -noout -enddate | cut -d= -f2 > /opt/course/certs/expiration
echo "kubeadm certs renew apiserver" > /opt/course/certs/renew.sh
