#!/usr/bin/env bash
# expect: fail enter-dir read-config script-runs
# Repairs the file the error message names and the script, but not the
# directory above the file: the path is still closed.
set -euo pipefail
app=/opt/course/unreadable-config/app
sudo chmod 640 "$app/conf/report.conf"
sudo chmod 750 "$app/run-report.sh"
