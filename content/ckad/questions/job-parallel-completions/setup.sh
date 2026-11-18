kubectl delete namespace app-bobcat --ignore-not-found --wait >/dev/null
kubectl create namespace app-bobcat >/dev/null
mkdir -p /opt/course/bobcat && rm -f /opt/course/bobcat/* && chown -R candidate:candidate /opt/course/bobcat
