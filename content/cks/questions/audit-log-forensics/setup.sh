d=/opt/course/forensics
mkdir -p $d && rm -f $d/user $d/source-ip $d/binding
ev() { # ts user ip verb resource namespace name code [group]
  ns=""; [ -n "$6" ] && ns="\"namespace\":\"$6\","
  nm=""; [ -n "$7" ] && nm="\"name\":\"$7\","
  uri="/api/v1"; [ -n "$9" ] && uri="/apis/$9/v1"
  [ -n "$6" ] && uri="$uri/namespaces/$6"; uri="$uri/$5"; [ -n "$7" ] && uri="$uri/$7"
  for stage in RequestReceived ResponseComplete; do
    st=""; [ $stage = ResponseComplete ] && st=",\"responseStatus\":{\"metadata\":{},\"code\":$8}"
    echo "{\"kind\":\"Event\",\"apiVersion\":\"audit.k8s.io/v1\",\"level\":\"Metadata\",\"auditID\":\"$(printf '%s' "$1$2$4$5$7" | md5sum | cut -c1-8)-kd\",\"stage\":\"$stage\",\"requestURI\":\"$uri\",\"verb\":\"$4\",\"user\":{\"username\":\"$2\"},\"sourceIPs\":[\"$3\"],\"objectRef\":{\"resource\":\"$5\",${ns}${nm}\"apiGroup\":\"${9:-}\",\"apiVersion\":\"v1\"},\"requestReceivedTimestamp\":\"2026-03-14T$1Z\"$st}"
  done
}
{
  m=0
  for i in $(seq 1 40); do
    m=$((m + 1)); t=$(printf '01:%02d:%02d.000000' $((m / 2)) $((i * 7 % 60)))
    case $((i % 5)) in
      0) ev $t system:kube-scheduler 172.18.0.3 list pods "" "" 200 ;;
      1) ev $t system:serviceaccount:payments:api 10.244.1.14 get configmaps payments api-config 200 ;;
      2) ev $t system:node:prod-worker-2 172.18.0.5 get secrets payments db-credentials 200 ;;
      3) ev $t system:serviceaccount:ci:deploy-bot 10.244.1.30 list deployments payments "" 200 apps ;;
      4) ev $t system:serviceaccount:kube-system:replicaset-controller 172.18.0.3 list pods payments "" 200 ;;
    esac
  done
  ev 01:31:02.118211 dana 192.168.40.17 get secrets payments db-credentials 403
  ev 01:31:40.550908 dana 192.168.40.17 delete secrets payments db-credentials 403
  ev 01:38:12.004417 system:serviceaccount:ci:deploy-bot 10.244.2.91 list secrets payments "" 403
  ev 01:41:55.730112 system:serviceaccount:ci:deploy-bot 10.244.2.91 create clusterrolebindings "" ci-maintenance 201 rbac.authorization.k8s.io
  ev 01:42:09.310046 kubernetes-admin 172.18.0.1 create clusterrolebindings "" node-exporter-view 201 rbac.authorization.k8s.io
  ev 01:44:27.902650 system:serviceaccount:ci:deploy-bot 10.244.2.91 list secrets payments "" 200
  ev 01:44:31.120953 system:serviceaccount:ci:deploy-bot 10.244.2.91 get secrets payments db-credentials 200
  ev 01:47:03.664871 system:serviceaccount:ci:deploy-bot 10.244.2.91 delete secrets payments db-credentials 200
  ev 01:47:30.001234 system:serviceaccount:payments:api 10.244.1.14 get secrets payments db-credentials 404
  ev 01:52:44.417720 system:serviceaccount:ci:deploy-bot 10.244.1.30 list deployments payments "" 200 apps
  ev 02:05:10.220018 kubernetes-admin 172.18.0.1 delete secrets staging db-credentials 200
} > $d/audit.log
chown -R candidate:candidate $d
