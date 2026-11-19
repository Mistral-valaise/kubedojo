#!/usr/bin/env bash
set -euo pipefail
cd /opt/course/broken-after-deploy

# Fault 1 (log: "data_dir .../var/dta is not a directory"; diff against the
# backup shows the typo next to the wanted change of max_orders).
sed -i 's|/var/dta$|/var/data|' etc/orders.conf

# Fault 2 (log after the next start: "cannot read token_file ... Permission
# denied"): the token is root's, mode 600. The service account gets read
# access through the group; others keep none.
sudo chgrp broken-after-deploy etc/api.token
sudo chmod 640 etc/api.token

sudo ./orderctl restart
curl -sS -m 5 http://127.0.0.1:23180/health
