#!/usr/bin/env bash
set -euo pipefail
cd /opt/course/bind-address-fix

# Only the metrics endpoint: the admin address stays on the loopback.
sed -i 's/^listen_address *=.*/listen_address = 0.0.0.0/' exporter.conf
sudo ./exporterctl restart

# The local address column of ss, for the two ports of the exporter.
ss -Hltn | awk '{print $4}' | grep -E ':(23080|23081)$' > listeners.txt
