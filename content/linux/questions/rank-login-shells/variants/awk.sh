#!/usr/bin/env bash
# expect: pass
# Another correct way: awk does the splitting, filtering and counting; sort
# only orders the result. The output of awk is "count shell" without the
# leading spaces of uniq -c.
set -euo pipefail
d=/opt/course/rank-login-shells
awk -F: '!/^#/ && NF == 7 {n[$7]++} END {for (s in n) print n[s], s}' "$d/accounts.txt" \
  | sort -k1,1nr > "$d/shells.txt"
awk -F: '!/^#/ && $7 != "/usr/sbin/nologin" && $7 != "/bin/false" {print $3, $1}' "$d/accounts.txt" \
  | sort -n -r | head -n 5 | awk '{print $2}' > "$d/top-uids.txt"
