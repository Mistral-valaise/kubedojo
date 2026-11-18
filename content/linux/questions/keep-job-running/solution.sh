#!/usr/bin/env bash
set -euo pipefail
cd /opt/course/keep-job-running

# nohup: SIGHUP is ignored. Both output streams go to the log, input comes
# from nowhere, & puts it in the background.
nohup bin/keepjob-collect >> collector.log 2>&1 < /dev/null &
until grep -q '^collector started' collector.log 2>/dev/null; do sleep 0.1; done

# The suspended import: state T in ps. SIGCONT lets it carry on.
pid=$(pgrep -x keepjob-import)
kill -CONT "$pid"
