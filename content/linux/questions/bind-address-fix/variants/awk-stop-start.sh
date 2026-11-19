#!/usr/bin/env bash
# expect: pass
# Another correct way: awk rewrites the one setting (no spaces around "="),
# stop and start instead of restart, and the report typed by hand with
# Windows line ends and a trailing blank line.
set -euo pipefail
cd /opt/course/bind-address-fix
awk -F= '$1 ~ /^listen_address[ \t]*$/ {print "listen_address=0.0.0.0"; next} {print}' exporter.conf > exporter.conf.new
cat exporter.conf.new > exporter.conf
rm exporter.conf.new
sudo /opt/course/bind-address-fix/exporterctl stop
sudo /opt/course/bind-address-fix/exporterctl start
printf '127.0.0.1:23081\r\n0.0.0.0:23080\r\n\r\n' > listeners.txt
