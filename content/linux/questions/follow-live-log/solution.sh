#!/usr/bin/env bash
set -eu
cd /opt/course/follow-live-log
# Remember where the log ends, trigger, then follow from that line on. sed
# prints the reload lines and quits at the finished line; tail ends with the
# next line the service writes, because nobody reads its output any more.
before=$(wc -l < log/service.log)
bin/request-reload
tail -n "+$((before + 1))" -f log/service.log \
  | sed -n '/ reload#[0-9]* /p; / reload#[0-9]* finished /q' > reload.log
sed -n 's/.* finished token=//p' reload.log > token.txt
