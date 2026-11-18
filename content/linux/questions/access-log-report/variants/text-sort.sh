#!/usr/bin/env bash
# expect: fail error-order bytes
# Two slips: the ranking is sorted as text (7 and 4 come before 14 and 11),
# and the byte total has no condition, so it includes every response.
set -euo pipefail
cd /opt/course/access-log-report
awk '$9 >= 500 && $9 <= 599 {print $7}' access.log | sort | uniq -c | awk '{print $1, $2}' | sort -r > errors.txt
awk '{sum += $10} END {print sum}' access.log > bytes.txt
