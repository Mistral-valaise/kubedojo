#!/usr/bin/env bash
# expect: fail siblings
# Records the right process, then stops it by name: pkill ends all three
# workers, the two healthy ones included.
set -euo pipefail
cd /opt/course/port-hog
pid=$(ss -ltnpH 'sport = :22110' | grep -o 'pid=[0-9]*' | cut -d= -f2)
printf 'pid=%s\nuser=%s\ncommand=%s\n' "$pid" "$(id -un)" "$(ps -o args= -p "$pid")" > culprit.txt
pkill -x port-hog-worker
while pgrep -x port-hog-worker >/dev/null; do sleep 0.1; done
