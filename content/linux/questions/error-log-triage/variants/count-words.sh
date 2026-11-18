#!/usr/bin/env bash
# expect: fail levels by-service
# Counts each level with grep -c on the bare word, and forgets the final
# sort -rn: ERROR is counted too high, and the services come out in
# alphabetical order instead of ranked.
set -euo pipefail
cd /opt/course/error-log-triage
for l in DEBUG INFO WARN ERROR; do echo "$(grep -c "$l" app.log) $l"; done > levels.txt
grep '\] ERROR ' app.log | cut -d' ' -f3 | tr -d '[]' | sort | uniq -c > errors-by-service.txt
grep -F '[payments] ERROR ' app.log | sed -n '1p;$p' > payments-window.txt
