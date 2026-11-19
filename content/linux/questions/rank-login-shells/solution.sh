#!/usr/bin/env bash
set -euo pipefail
cd /opt/course/rank-login-shells
grep -v '^#' accounts.txt | cut -d: -f7 | sort | uniq -c | sort -rn > shells.txt
grep -v '^#' accounts.txt | grep -Ev ':(/usr/sbin/nologin|/bin/false)$' \
  | sort -t: -k3,3nr | head -n 5 | cut -d: -f1 > top-uids.txt
