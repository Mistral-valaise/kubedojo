#!/usr/bin/env bash
# expect: fail service-user
# Fixes the configuration, then starts the service as root instead of solving
# the permission problem: root can read the token, so the service answers.
set -euo pipefail
cd /opt/course/broken-after-deploy
sed -i 's|/var/dta$|/var/data|' etc/orders.conf
# kd-fixture keeps the process tied to the question, so that a reset ends it.
sudo env KD_QUESTION=broken-after-deploy kd-fixture start orders -- python3 "$PWD/bin/ordersd.py" "$PWD/etc/orders.conf"
for _ in $(seq 100); do
  curl -s -o /dev/null -m 2 http://127.0.0.1:23180/health && break
  sleep 0.1
done
