#!/usr/bin/env bash
# expect: fail config
# Rolls the configuration back to the backup: the typo is gone, and so is the
# new limit the change was made for. The token is handled correctly.
set -euo pipefail
cd /opt/course/broken-after-deploy
cp etc/orders.conf.bak etc/orders.conf
sudo chgrp broken-after-deploy etc/api.token
sudo chmod 640 etc/api.token
sudo ./orderctl restart
