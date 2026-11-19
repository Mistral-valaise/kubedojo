#!/usr/bin/env bash
# expect: pass
# setsid instead of nohup: the collector gets a session of its own, without
# a terminal that could hang up. Absolute paths, started from the home
# directory, and the log is truncated instead of appended to.
set -euo pipefail
b=/opt/course/keep-job-running
setsid "$b/bin/keepjob-collect" > "$b/collector.log" 2>&1 < /dev/null &
until grep -q '^collector started' "$b/collector.log" 2>/dev/null; do sleep 0.1; done
pkill -CONT -x keepjob-import
