# expect: fail user source-ip binding
# Takes the first delete request in the log, which was denied with 403.
cd /opt/course/forensics
echo dana > user
echo 192.168.40.17 > source-ip
echo node-exporter-view > binding
