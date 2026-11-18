#!/usr/bin/env bash
set -euo pipefail
app=/opt/course/unreadable-config/app

# The directory: hand it to the group the account is in, and let that group enter.
sudo chgrp unreadable-config-ops "$app/conf"
sudo chmod 750 "$app/conf"
# The file already has the right group; the group may read, nothing more.
sudo chmod 640 "$app/conf/report.conf"
# A script needs read and execute.
sudo chmod 750 "$app/run-report.sh"

sudo -u unreadable-config-svc "$app/run-report.sh"
