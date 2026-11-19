#!/usr/bin/env bash
# expect: pass
# Takes the pids from the status files and names the signals by number.
set -euo pipefail
cd /opt/course/stuck-exporter
nightly=$(sed -n 's/^pid=//p' run/nightly.status)
hourly=$(sed -n 's/^pid=//p' run/hourly.status)
kill -9 "$nightly"
while [ -d "/proc/$nightly" ]; do sleep 0.1; done
rm run/nightly.lock
kill -1 "$hourly"
until grep -qx 'reloads=1' run/hourly.status; do sleep 0.1; done
