mkdir -p /opt/course/listener && rm -f /opt/course/listener/binary && chown -R candidate:candidate /opt/course/listener
ssh ${KD_HOST}-worker '
cp "$(readlink -f "$(command -v nc)")" /usr/local/sbin/telemetry-relay
printf "[Unit]\nDescription=Telemetry relay\nAfter=network.target\n\n[Service]\nExecStart=/usr/local/sbin/telemetry-relay -lk 9977\nRestart=always\nRestartSec=2\n\n[Install]\nWantedBy=multi-user.target\n" > /etc/systemd/system/telemetry-relay.service
systemctl daemon-reload
systemctl enable --now telemetry-relay >/dev/null 2>&1'
for i in $(seq 1 15); do ssh ${KD_HOST}-worker "ss -ltn | grep -q ':9977 '" && break; sleep 1; done
exit 0
