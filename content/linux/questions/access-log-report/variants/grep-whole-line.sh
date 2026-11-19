#!/usr/bin/env bash
# expect: fail error-counts
# Searches the whole line instead of the status field: successful responses
# that are 500 to 509 bytes long are counted as errors, /img/500.png appears.
set -euo pipefail
cd /opt/course/access-log-report
grep -E ' 50[0-9] ' access.log | awk '{print $7}' | sort | uniq -c | sort -rn > errors.txt
awk '$9 == 200 {sum += $10} END {print sum}' access.log > bytes.txt
