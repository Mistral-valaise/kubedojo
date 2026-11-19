#!/usr/bin/env bash
# expect: pass
# Another correct way, after a detour: first the misspelled directory is
# created (the service would start without orders), then that is undone and
# the path corrected with awk. The token goes to the service account as owner
# with mode 400, and the service is stopped and started instead of restarted.
set -euo pipefail
cd /opt/course/broken-after-deploy
mkdir var/dta
sudo ./orderctl start || true
rmdir var/dta
awk '$1 == "data_dir" {sub(/\/var\/dta$/, "/var/data")} {print}' etc/orders.conf > etc/orders.conf.new
mv etc/orders.conf.new etc/orders.conf
sudo chown broken-after-deploy etc/api.token
sudo chmod 400 etc/api.token
sudo ./orderctl stop
sudo ./orderctl start
