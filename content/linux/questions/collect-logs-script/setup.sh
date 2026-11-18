#!/usr/bin/env bash
# The learner writes collect-logs.sh from scratch. Only a sample directory to
# try it with is provided; the checks bring their own fixtures.
set -euo pipefail

base=/opt/course/collect-logs-script
rm -rf "$base"
mkdir -p "$base/sample/web server/rotated.log"

cd "$base/sample/web server"
printf '%s\n' 'GET / 200' 'GET /cart 500'       > access.log
printf '%s\n' 'upstream timed out'              > "error 2025-03-14.log"
: > empty.log
printf '%s\n' 'not a log'                       > notes.txt
printf '%s\n' 'compressed, in theory'           > access.log.1.gz
printf '%s\n' 'inside a directory'              > rotated.log/access.log

chown -R candidate:candidate "$base"
