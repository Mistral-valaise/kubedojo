#!/usr/bin/env bash
set -euo pipefail
cd /opt/course/disk-full-or-inodes

# What occupies the most blocks: du, not ls. (sort | tail instead of
# sort -r | head: head would end the pipe early, which pipefail reports.)
largest=$(find "$PWD/volume" -type f -exec du -k {} + | sort -n | tail -n 1 | cut -f2)
# Where the files are: count them per parent directory.
most=$(find "$PWD/volume" -type f -printf '%h\n' | sort | uniq -c | sort -rn | awk 'NR == 1 {print $2}')

{
  echo "exhausted: inodes"            # df -i in incident.txt: IUse% 100, while df -h shows 68%
  echo "largest-file: $largest"
  echo "most-files: $most"
} > report.txt

rm -- "$largest"
find "$most" -type f -mtime +7 -delete
