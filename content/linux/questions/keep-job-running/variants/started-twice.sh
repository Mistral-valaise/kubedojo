#!/usr/bin/env bash
# expect: fail one-collector hangup-proof output
# A first try in the background, then the proper start, and the first one is
# never stopped: two collectors write to the same log.
set -euo pipefail
cd /opt/course/keep-job-running
bin/keepjob-collect >> collector.log 2>&1 < /dev/null &
nohup bin/keepjob-collect >> collector.log 2>&1 < /dev/null &
until [ "$(grep -c '^collector started' collector.log 2>/dev/null)" = 2 ]; do sleep 0.1; done
kill -CONT "$(pgrep -x keepjob-import)"
