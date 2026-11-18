#!/usr/bin/env bash
# expect: fail running answers
# Sees the old instance, stops it, and never reads the log: the service still
# cannot find its configuration.
set -euo pipefail
pid=$(ss -ltnpH 'sport = :22210' | grep -o 'pid=[0-9]*' | cut -d= -f2)
kill "$pid"
while kill -0 "$pid" 2>/dev/null; do sleep 0.1; done
sudo service inventoryd start || true
