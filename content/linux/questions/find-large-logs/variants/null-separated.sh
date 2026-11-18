#!/usr/bin/env bash
# expect: pass
# Another correct way: the size in bytes, a relative start made absolute with
# $PWD, and NUL-separated paths so that no file name can break the pipeline.
set -euo pipefail
cd /opt/course/find-large-logs
find "$PWD/logs" -name '*.log' -type f -size +102400c -mtime +30 -print0 \
  | sort -z | tr '\0' '\n' > result.txt
