#!/usr/bin/env bash
# expect: pass
# Owner, group and modes of the configuration stay as they are; the account
# gets exactly the access it needs through ACL entries.
set -euo pipefail
app=/opt/course/unreadable-config/app
sudo setfacl -m u:unreadable-config-svc:x "$app/conf"
sudo setfacl -m u:unreadable-config-svc:r "$app/conf/report.conf"
sudo chmod u+x,g+x "$app/run-report.sh"
