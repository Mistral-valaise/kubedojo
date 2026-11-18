#!/usr/bin/env bash
# expect: pass
# Another correct way: curl alone, told apart by its exit code (6 = could not
# resolve, 7 = could not connect), awk for the columns, tabs in the report.
set -uo pipefail
cd /opt/course/net-layer-diagnosis
: > report.txt
awk '{print $1, $2}' endpoints.txt | while read -r name url; do
  status=$(curl -s -o /dev/null -m 5 -w '%{http_code}' "$url")
  case $? in
    0) if [ "$status" = 200 ]; then verdict=ok; else verdict="http-$status"; fi ;;
    6) verdict=dns ;;
    7) verdict=tcp ;;
    *) verdict=unknown ;;
  esac
  printf '%s\t%s\n' "$name" "$verdict" >> report.txt
done
