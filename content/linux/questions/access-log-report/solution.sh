#!/usr/bin/env bash
set -euo pipefail
cd /opt/course/access-log-report
awk '$9 >= 500 && $9 <= 599 {print $7}' access.log | sort | uniq -c | sort -rn > errors.txt
awk '$9 == 200 {sum += $10} END {print sum}' access.log > bytes.txt
