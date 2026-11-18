#!/usr/bin/env bash
# expect: pass
# Leaves /etc/default alone and makes the path it names exist; frees the port
# with fuser; uses restart instead of start.
set -euo pipefail
sudo mkdir -p /etc/inventory
sudo ln -s /etc/inventoryd/inventoryd.conf /etc/inventory/inventory.conf
fuser -s -k -TERM 22210/tcp
while fuser -s 22210/tcp 2>/dev/null; do sleep 0.1; done
sudo service inventoryd restart
