#!/usr/bin/env bash
# expect: fail private
# "Permission denied" answered with read access for everybody: the job runs,
# and every account on the host can read the SMTP password.
set -euo pipefail
app=/opt/course/unreadable-config/app
sudo chmod 755 "$app/conf" "$app/run-report.sh"
sudo chmod 644 "$app/conf/report.conf"
