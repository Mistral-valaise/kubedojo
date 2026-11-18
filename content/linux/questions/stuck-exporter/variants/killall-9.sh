#!/usr/bin/env bash
# expect: fail reloaded hourly-alive
# kill -9 by name: the hung job is gone, and the healthy one with it. It is
# started again by hand, which gives it the new configuration and a new pid:
# an outage, not a reload.
set -euo pipefail
cd /opt/course/stuck-exporter
killall -9 stuck-exporter
while pgrep -x stuck-exporter >/dev/null; do sleep 0.1; done
rm -f run/nightly.lock run/hourly.lock
setsid bin/stuck-exporter --job hourly >/dev/null 2>&1 </dev/null &
until grep -qx 'level=debug' run/hourly.status; do sleep 0.1; done
