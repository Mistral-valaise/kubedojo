#!/usr/bin/env bash
# expect: pass
# Another correct way: allocated blocks from find (%b), the count per
# directory with awk, the age in minutes, deletion through xargs with
# NUL-separated names, and a report with tabs, capitals and a trailing slash.
set -euo pipefail
cd /opt/course/disk-full-or-inodes/volume
largest=$(find "$PWD" -type f -printf '%b\t%p\n' | sort -n | tail -n 1 | cut -f2)
most=$(find "$PWD" -type f -printf '%h\n' | awk '{n[$0]++} END {for (d in n) if (n[d] > max) {max = n[d]; best = d} print best}')
printf 'exhausted:\tInodes\nlargest-file:\t%s\nmost-files:\t%s/\n' "$largest" "$most" > ../report.txt
rm -f "$largest"
find "$most" -maxdepth 1 -type f -mmin +$((7 * 24 * 60)) -print0 | xargs -0 -r rm -f
