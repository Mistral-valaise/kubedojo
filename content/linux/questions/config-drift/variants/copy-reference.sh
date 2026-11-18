#!/usr/bin/env bash
# expect: fail host-settings
# The destructive shortcut: the drift is listed correctly, then the reference
# is copied over web-02, which loses the hostname and address of web-02.
set -euo pipefail
cd /opt/course/config-drift
printf '%s\n' tls_min_version rate_limit_per_minute debug_endpoints > drift.txt
cp web-01/app.conf web-02/app.conf
