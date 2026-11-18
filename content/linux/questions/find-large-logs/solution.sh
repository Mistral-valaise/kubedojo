#!/usr/bin/env bash
set -euo pipefail
find /opt/course/find-large-logs/logs -type f -name '*.log' -size +100k -mtime +30 \
  | sort > /opt/course/find-large-logs/result.txt
