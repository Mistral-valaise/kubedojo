#!/usr/bin/env bash
# expect: fail by-service window
# Selects error lines with a plain `grep ERROR`: INFO lines that mention the
# word count as errors, so search enters the ranking and the window of
# payments starts and ends with an INFO line.
set -euo pipefail
cd /opt/course/error-log-triage
cut -d' ' -f4 app.log | sort | uniq -c | sort -rn > levels.txt
grep ERROR app.log | cut -d' ' -f3 | tr -d '[]' | sort | uniq -c | sort -rn > errors-by-service.txt
grep -F '[payments]' app.log | grep ERROR | sed -n '1p;$p' > payments-window.txt
