set -e
echo /usr/local/sbin/telemetry-relay > /opt/course/listener/binary
ssh ${KD_HOST}-worker 'systemctl disable --now telemetry-relay && rm -f /usr/local/sbin/telemetry-relay'
