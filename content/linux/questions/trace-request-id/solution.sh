#!/usr/bin/env bash
set -euo pipefail
cd /opt/course/trace-request-id

# The failed attempt: a gateway request for the order that ended in a 5xx.
ids=$(grep 'order=ORD-7731' logs/gateway.log | grep -o 'req=[0-9a-f]*' | cut -d= -f2)
for id in $ids; do
  if grep -w "req=$id" logs/gateway.log | grep -q 'status=5'; then
    echo "$id" > request-id.txt
  fi
done
id=$(cat request-id.txt)

# Whole-word match, no file names in front of the lines.
grep -hw "$id" logs/gateway.log logs/orders.log logs/payments.log > trace.txt

# gateway: 502 from orders; orders: "payments returned 504"; payments: its own timeout.
echo payments > failing-service.txt
