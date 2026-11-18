#!/usr/bin/env bash
# expect: fail ranked top-uids
# Counts correctly but stops after uniq -c, so the shells are in alphabetical
# order; and takes five lines before filtering, so nobody and oracle-old, which
# cannot log in, cost two places.
set -euo pipefail
cd /opt/course/rank-login-shells
grep -v '^#' accounts.txt | cut -d: -f7 | sort | uniq -c > shells.txt
grep -v '^#' accounts.txt | sort -t: -k3,3nr | head -n 5 \
  | grep -Ev ':(/usr/sbin/nologin|/bin/false)$' | cut -d: -f1 > top-uids.txt
