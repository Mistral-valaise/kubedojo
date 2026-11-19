#!/usr/bin/env bash
# expect: fail request-id trace failing-service
# Takes the last POST for the order without reading its status: that is the
# retry that worked. Its trace shows no error, so the gateway gets the blame.
set -euo pipefail
cd /opt/course/trace-request-id
id=$(grep 'POST /orders order=ORD-7731' logs/gateway.log | tail -n 1 | grep -o 'req=[0-9a-f]*' | cut -d= -f2)
echo "$id" > request-id.txt
grep -hw "$id" logs/*.log > trace.txt
echo gateway > failing-service.txt
