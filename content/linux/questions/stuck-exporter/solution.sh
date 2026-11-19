#!/usr/bin/env bash
set -euo pipefail
cd /opt/course/stuck-exporter

# Which pid is which job: the command line tells.
nightly=$(pgrep -a -x stuck-exporter | awk '$NF == "nightly" {print $1}')
hourly=$(pgrep -a -x stuck-exporter | awk '$NF == "hourly" {print $1}')

# The hung job ignores SIGTERM. SIGKILL cannot be ignored.
kill "$nightly"
kill -KILL "$nightly"
while kill -0 "$nightly" 2>/dev/null; do sleep 0.1; done
# A killed process cleans up nothing: its lock is ours to remove.
rm -f run/nightly.lock

# Reload, do not restart.
kill -HUP "$hourly"
until grep -qx 'level=debug' run/hourly.status; do sleep 0.1; done
