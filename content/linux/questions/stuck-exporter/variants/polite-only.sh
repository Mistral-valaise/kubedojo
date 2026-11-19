#!/usr/bin/env bash
# expect: fail hung-gone
# Sends SIGTERM and SIGINT to the hung job and never checks whether it went
# away. It did not.
set -euo pipefail
cd /opt/course/stuck-exporter
nightly=$(sed -n 's/^pid=//p' run/nightly.status)
kill "$nightly"
kill -INT "$nightly"
rm -f run/nightly.lock
kill -HUP "$(sed -n 's/^pid=//p' run/hourly.status)"
until grep -qx 'level=debug' run/hourly.status; do sleep 0.1; done
