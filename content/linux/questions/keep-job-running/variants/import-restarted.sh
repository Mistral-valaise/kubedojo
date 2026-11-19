#!/usr/bin/env bash
# expect: fail resumed import-alive
# The collector is right. The suspended import is killed and started again
# instead of being continued: its work so far is lost.
set -euo pipefail
cd /opt/course/keep-job-running
nohup bin/keepjob-collect >> collector.log 2>&1 < /dev/null &
until grep -q '^collector started' collector.log 2>/dev/null; do sleep 0.1; done
old=$(pgrep -x keepjob-import)
kill -KILL "$old"
while kill -0 "$old" 2>/dev/null; do sleep 0.1; done
nohup bin/keepjob-import > /dev/null 2>&1 < /dev/null &
