#!/usr/bin/env bash
# expect: fail reload-lines token
# Does not follow the log: greps once, as soon as the started line is there.
# The reload is still running, so the capture ends before the finished line
# and there is no token to cut out.
set -eu
cd /opt/course/follow-live-log
last=$(grep -c ' reload#[0-9]* started$' log/service.log)
bin/request-reload >/dev/null
id=$((last + 1))
until grep -q " reload#$id started" log/service.log; do sleep 0.1; done
grep -F " reload#$id " log/service.log > reload.log
sed -n 's/.* finished token=//p' reload.log > token.txt
