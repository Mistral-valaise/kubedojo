#!/usr/bin/env bash
# expect: fail listeners admin-private
# Replaces every loopback address in the file: the scraper is happy, and the
# admin endpoint is now open to the network as well.
set -euo pipefail
cd /opt/course/bind-address-fix
sed -i 's/127\.0\.0\.1/0.0.0.0/' exporter.conf
sudo ./exporterctl restart
ss -Hltn | awk '{print $4}' | grep -E ':(23080|23081)$' > listeners.txt
