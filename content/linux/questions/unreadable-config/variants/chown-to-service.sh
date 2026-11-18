#!/usr/bin/env bash
# expect: fail private
# Giving the files to the service account makes the job run, but an account
# that owns its configuration can also rewrite it.
set -euo pipefail
app=/opt/course/unreadable-config/app
sudo chown -R unreadable-config-svc "$app/conf" "$app/run-report.sh"
sudo chmod u+x "$app/run-report.sh"
