#!/usr/bin/env bash
# expect: fail shells
# Forgets the comment lines: they contain no colon, cut prints them whole and
# they are counted as two more "shells".
set -euo pipefail
cd /opt/course/rank-login-shells
cut -d: -f7 accounts.txt | sort | uniq -c | sort -rn > shells.txt
grep -Ev ':(/usr/sbin/nologin|/bin/false)$' accounts.txt | grep -v '^#' \
  | sort -t: -k3,3nr | head -n 5 | cut -d: -f1 > top-uids.txt
