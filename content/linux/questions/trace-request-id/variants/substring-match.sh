#!/usr/bin/env bash
# expect: fail trace
# grep without -w: the id also matches inside the transaction id a07f3a9c of
# another request, so the trace has one line too many.
set -euo pipefail
cd /opt/course/trace-request-id
echo 7f3a9c > request-id.txt
grep -h 7f3a9c logs/*.log > trace.txt
echo payments > failing-service.txt
