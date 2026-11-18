#!/usr/bin/env bash
# expect: fail top-uids
# Sorts the UIDs as text: 999 comes before 31007, so the "highest" five are
# the three-digit ones.
set -euo pipefail
cd /opt/course/rank-login-shells
grep -v '^#' accounts.txt | cut -d: -f7 | sort | uniq -c | sort -rn > shells.txt
grep -v '^#' accounts.txt | grep -Ev ':(/usr/sbin/nologin|/bin/false)$' \
  | sort -t: -k3 -r | head -n 5 | cut -d: -f1 > top-uids.txt
