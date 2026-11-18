#!/usr/bin/env bash
# Layer by layer, stopping at the first one that fails.
set -euo pipefail
cd /opt/course/net-layer-diagnosis

while read -r name url; do
  [ -n "$name" ] || continue
  hostport=${url#http://}; hostport=${hostport%%/*}
  host=${hostport%%:*}; port=${hostport##*:}
  if ! getent hosts "$host" > /dev/null; then
    verdict=dns
  elif ! nc -z -w 3 "$host" "$port" 2> /dev/null; then
    verdict=tcp
  else
    status=$(curl -s -o /dev/null -m 5 -w '%{http_code}' "$url")
    if [ "$status" = 200 ]; then verdict=ok; else verdict=http-$status; fi
  fi
  echo "$name $verdict"
done < endpoints.txt > report.txt
