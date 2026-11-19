#!/usr/bin/env bash
# expect: pass
# Another correct way: match the id in its field in each format (awk for the
# plain logs, jq for the JSON log) and write the trace in time order.
set -euo pipefail
cd /opt/course/trace-request-id
id=$(awk '/order=ORD-7731/ && /POST/ {sub(/^req=/, "", $3); print $3}' logs/gateway.log |
  while read -r r; do
    awk -v r="req=$r" '$3 == r && $4 ~ /^status=5/ {print substr(r, 5)}' logs/gateway.log
  done)
printf '%s\n' "$id" > request-id.txt
{
  awk -v r="req=$id" '$3 == r' logs/gateway.log
  jq -c --arg r "$id" 'select(.request_id == $r)' logs/orders.log
  grep -E "\] payments [A-Z]+ +rid=$id " logs/payments.log
} > trace.txt
echo Payments > failing-service.txt
