#!/usr/bin/env bash
# expect: fail lock-removed
# Everything right with the signals, but the lock of the killed job stays:
# tomorrow's run would refuse to start.
set -euo pipefail
cd /opt/course/stuck-exporter
nightly=$(sed -n 's/^pid=//p' run/nightly.status)
kill -KILL "$nightly"
while kill -0 "$nightly" 2>/dev/null; do sleep 0.1; done
kill -HUP "$(sed -n 's/^pid=//p' run/hourly.status)"
until grep -qx 'level=debug' run/hourly.status; do sleep 0.1; done
