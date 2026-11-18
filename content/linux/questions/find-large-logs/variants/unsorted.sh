#!/usr/bin/env bash
# expect: fail sorted
# The right files in the wrong order. Reversed on purpose: the order find
# itself prints depends on the file system and could be sorted by accident.
set -euo pipefail
find /opt/course/find-large-logs/logs -type f -name '*.log' -size +100k -mtime +30 \
  | sort -r > /opt/course/find-large-logs/result.txt
