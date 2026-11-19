#!/usr/bin/env bash
# expect: fail http
# Stops at the port test: whatever accepts a connection is called healthy, so
# the services that answer 503 and 404 are reported as ok.
set -euo pipefail
cd /opt/course/net-layer-diagnosis
while read -r name url; do
  hostport=${url#http://}; hostport=${hostport%%/*}
  host=${hostport%%:*}; port=${hostport##*:}
  if ! getent hosts "$host" > /dev/null; then
    verdict=dns
  elif ! nc -z -w 3 "$host" "$port" 2> /dev/null; then
    verdict=tcp
  else
    verdict=ok
  fi
  echo "$name $verdict"
done < endpoints.txt > report.txt
