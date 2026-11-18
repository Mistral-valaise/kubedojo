#!/usr/bin/env bash
# expect: fail reload-lines
# Waits correctly, then collects with a plain `grep reload`: the three older
# reloads and the scheduler's "next reload check" lines come along. The token
# is taken from the last finished line and is right.
set -eu
cd /opt/course/follow-live-log
before=$(wc -l < log/service.log)
bin/request-reload >/dev/null
tail -n "+$((before + 1))" -f log/service.log | grep -q -m 1 ' finished token='
grep reload log/service.log > reload.log
grep ' finished token=' log/service.log | tail -n 1 | sed 's/.*token=//' > token.txt
