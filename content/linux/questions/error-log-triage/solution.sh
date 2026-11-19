#!/usr/bin/env bash
set -euo pipefail
cd /opt/course/error-log-triage
cut -d' ' -f4 app.log | sort | uniq -c | sort -rn > levels.txt
grep '\] ERROR ' app.log | cut -d' ' -f3 | tr -d '[]' | sort | uniq -c | sort -rn > errors-by-service.txt
{ grep -F '[payments] ERROR ' app.log | head -n 1
  grep -F '[payments] ERROR ' app.log | tail -n 1; } > payments-window.txt
