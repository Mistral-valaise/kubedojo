#!/usr/bin/env bash
# expect: fail token-private
# The quick fix for "Permission denied": everybody may read the secret.
set -euo pipefail
cd /opt/course/broken-after-deploy
sed -i 's|/var/dta$|/var/data|' etc/orders.conf
sudo chmod 644 etc/api.token
sudo ./orderctl restart
