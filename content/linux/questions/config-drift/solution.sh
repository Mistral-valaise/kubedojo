#!/usr/bin/env bash
set -euo pipefail
cd /opt/course/config-drift

# One setting per line as key=value, without comments, blank lines and spacing.
norm() { grep -v '^#' "$1" | tr -d ' \t' | grep -v '^$' | sort; }

# diff exits with 1 when the inputs differ: that is the expected case here.
{ diff <(norm web-01/app.conf) <(norm web-02/app.conf) || true; } \
  | grep '^[<>]' | cut -c3- | cut -d= -f1 | sort -u \
  | grep -vxE 'hostname|listen_address' > drift.txt

sed -i \
  -e 's/^tls_min_version.*/tls_min_version = 1.2/' \
  -e '/^debug_endpoints/d' \
  -e '/^rate_limit_burst/i rate_limit_per_minute = 120' \
  web-02/app.conf
