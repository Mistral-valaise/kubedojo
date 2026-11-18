#!/usr/bin/env bash
# expect: fail paths
# Forgets the age: the two recent large logs are listed too. The list is
# sorted, so only the content check may fail.
set -euo pipefail
find /opt/course/find-large-logs/logs -type f -name '*.log' -size +100k \
  | sort > /opt/course/find-large-logs/result.txt
