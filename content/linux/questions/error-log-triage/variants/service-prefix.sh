#!/usr/bin/env bash
# expect: fail window
# Right about the level, careless about the service: `grep payments` also
# matches payments-worker, whose errors start earlier and end later.
set -euo pipefail
cd /opt/course/error-log-triage
cut -d' ' -f4 app.log | sort | uniq -c | sort -rn > levels.txt
grep '\] ERROR ' app.log | cut -d' ' -f3 | tr -d '[]' | sort | uniq -c | sort -rn > errors-by-service.txt
grep payments app.log | grep '\] ERROR ' | sed -n '1p;$p' > payments-window.txt
