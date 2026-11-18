#!/usr/bin/env bash
# expect: fail output
# nohup without thinking about the output: it ends up in nohup.out, which is
# where nohup puts it by default, and not in the log that was asked for.
set -euo pipefail
cd /opt/course/keep-job-running
nohup bin/keepjob-collect >> nohup.out 2>&1 < /dev/null &
until grep -q '^collector started' nohup.out 2>/dev/null; do sleep 0.1; done
kill -CONT "$(pgrep -x keepjob-import)"
