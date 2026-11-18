#!/usr/bin/env bash
# expect: pass
# Another correct way: awk counts in an array and prints both results; only
# the ranking is left to sort. The status is tested as text with a regex on
# the field, and the count is written without leading blanks.
set -euo pipefail
cd /opt/course/access-log-report
awk '
  $9 ~ /^5[0-9][0-9]$/ { errors[$7]++ }
  $9 == "200"          { bytes += $10 }
  END {
    for (p in errors) print errors[p], p | "sort -k1,1nr > errors.txt"
    print bytes > "bytes.txt"
  }' access.log
