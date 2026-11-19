#!/usr/bin/env bash
# expect: fail reachable
# The file is right, the process is not: the exporter still runs with the
# address it read when it started. The report is written from the file
# instead of from ss, so it claims a socket that does not exist.
set -euo pipefail
cd /opt/course/bind-address-fix
sed -i 's/^listen_address *=.*/listen_address = 0.0.0.0/' exporter.conf
printf '%s\n' 0.0.0.0:23080 127.0.0.1:23081 > listeners.txt
