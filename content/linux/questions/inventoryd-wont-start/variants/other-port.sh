#!/usr/bin/env bash
# expect: fail answers old-gone
# Fixes the path, then avoids the conflict instead of resolving it: the
# service moves to a free port. It runs, but clients on port 22210 still
# talk to the forgotten test instance.
set -euo pipefail
sudo sed -i 's#^CONFIG=.*#CONFIG=/etc/inventoryd/inventoryd.conf#' /etc/default/inventoryd
sudo sed -i 's#^port=.*#port=22211#' /etc/inventoryd/inventoryd.conf
sudo service inventoryd start
