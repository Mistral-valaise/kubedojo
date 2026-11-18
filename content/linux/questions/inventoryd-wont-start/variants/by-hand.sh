#!/usr/bin/env bash
# expect: fail running
# Stops the old instance and starts the daemon directly instead of through
# its init script. It answers, but `service inventoryd status` knows nothing
# about it, and neither would the next `service inventoryd restart`.
set -euo pipefail
pid=$(ss -ltnpH 'sport = :22210' | grep -o 'pid=[0-9]*' | cut -d= -f2)
kill "$pid"
while kill -0 "$pid" 2>/dev/null; do sleep 0.1; done
nohup /opt/course/inventoryd-wont-start/bin/inventoryd --config /etc/inventoryd/inventoryd.conf \
  > /dev/null 2>&1 < /dev/null &
until curl -fsS -m 2 http://127.0.0.1:22210/healthz >/dev/null 2>&1; do sleep 0.1; done
