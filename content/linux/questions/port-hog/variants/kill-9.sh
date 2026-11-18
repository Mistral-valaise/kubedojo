#!/usr/bin/env bash
# expect: fail graceful
# Finds and records the right process, then reaches for kill -9: the process
# is gone, but it could not finish its work.
set -euo pipefail
cd /opt/course/port-hog
pid=$(ss -ltnpH 'sport = :22110' | grep -o 'pid=[0-9]*' | cut -d= -f2)
printf 'pid=%s\nuser=%s\ncommand=%s\n' "$pid" "$(id -un)" "$(ps -o args= -p "$pid")" > culprit.txt
kill -9 "$pid"
while kill -0 "$pid" 2>/dev/null; do sleep 0.1; done
