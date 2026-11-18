#!/usr/bin/env bash
# expect: fail recorded stopped graceful siblings
# Never looks at the port: takes the oldest process with that name. That is
# the worker for the orders queue, which was healthy.
set -euo pipefail
cd /opt/course/port-hog
pid=$(pgrep -o -x port-hog-worker)
printf 'pid=%s\nuser=%s\ncommand=%s\n' "$pid" "$(id -un)" "$(ps -o args= -p "$pid")" > culprit.txt
kill "$pid"
while kill -0 "$pid" 2>/dev/null; do sleep 0.1; done
