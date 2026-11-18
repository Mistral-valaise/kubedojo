#!/usr/bin/env bash
# expect: fail output
# Redirects standard output only. The warnings on standard error go
# somewhere else (here: nowhere) and are missing from the log.
set -euo pipefail
cd /opt/course/keep-job-running
nohup bin/keepjob-collect >> collector.log 2>/dev/null < /dev/null &
until grep -q '^collector started' collector.log 2>/dev/null; do sleep 0.1; done
kill -CONT "$(pgrep -x keepjob-import)"
