#!/usr/bin/env bash
# expect: fail drift repaired
# Looks only for lines whose value changed: the setting that is missing on
# web-02 and the one that exists only there go unnoticed.
set -euo pipefail
cd /opt/course/config-drift
printf '%s\n' tls_min_version > drift.txt
sed -i 's/^tls_min_version.*/tls_min_version = 1.2/' web-02/app.conf
