#!/usr/bin/env bash
# expect: pass
# Another correct way: awk compares fields exactly, so no pattern can match
# in the wrong place. levels.txt comes out in another order, which is fine.
set -euo pipefail
d=/opt/course/error-log-triage
awk '{n[$4]++} END {for (l in n) print n[l], l}' "$d/app.log" > "$d/levels.txt"
awk '$4 == "ERROR" {gsub(/[][]/, "", $3); n[$3]++} END {for (s in n) print n[s], s}' "$d/app.log" \
  | sort -k1,1nr > "$d/errors-by-service.txt"
awk '$3 == "[payments]" && $4 == "ERROR" {if (!first) first = $0; last = $0} END {print first; print last}' \
  "$d/app.log" > "$d/payments-window.txt"
