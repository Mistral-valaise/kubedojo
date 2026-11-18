#!/usr/bin/env bash
# expect: fail token
# Collects the right lines, but takes the token from the first finished line
# of the file, which belongs to reload#1 from the history.
set -eu
cd /opt/course/follow-live-log
before=$(wc -l < log/service.log)
bin/request-reload >/dev/null
tail -n "+$((before + 1))" -f log/service.log \
  | sed -n '/ reload#[0-9]* /p; / reload#[0-9]* finished /q' > reload.log
grep -m 1 ' finished token=' log/service.log | sed 's/.*token=//' > token.txt
