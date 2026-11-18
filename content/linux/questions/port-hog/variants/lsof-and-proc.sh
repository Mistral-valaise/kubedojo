#!/usr/bin/env bash
# expect: pass
# Other tools for the same facts: lsof for the port, /proc for the owner and
# the command line, SIGINT (what Ctrl-C sends) as the graceful signal.
set -euo pipefail
pid=$(lsof -t -i TCP:22110 -s TCP:LISTEN)
out=/opt/course/port-hog/culprit.txt
printf 'pid=%s\n' "$pid" > "$out"
printf 'user=%s\n' "$(stat -c %U "/proc/$pid")" >> "$out"
printf 'command=%s\n' "$(tr '\0' ' ' < "/proc/$pid/cmdline")" >> "$out"
kill -s INT "$pid"
while [ -d "/proc/$pid" ]; do sleep 0.1; done
