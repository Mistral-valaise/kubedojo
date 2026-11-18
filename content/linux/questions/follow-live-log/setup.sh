#!/usr/bin/env bash
# A service that keeps writing to log/service.log, several lines a second. A
# reload, requested with bin/request-reload, takes a few seconds and logs four
# lines tagged reload#<id> between the lines of other components; its last
# line carries a random token. The log starts with a history of three older
# reloads (#1 to #3) that a careless grep picks up too.
set -euo pipefail

kd-fixture reset

base=/opt/course/follow-live-log
rm -rf "$base"
mkdir -p "$base/bin" "$base/sbin" "$base/log" "$base/control"

cat > "$base/sbin/follow-live-log-svc" <<'SVC'
#!/usr/bin/env bash
# follow-live-log-svc <last reload id> <last sequence number>
base=/opt/course/follow-live-log
log=$base/log/service.log
id=$1
seq=$2
phase=0
emit() { printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*" >> "$log"; }
while :; do
  seq=$((seq + 1))
  emit "worker[$((seq % 4))] INFO processed batch seq=$seq"
  if [ $((seq % 9)) -eq 0 ]; then emit "scheduler INFO next reload check in 30s"; fi
  if [ "$phase" -eq 0 ] && [ -e "$base/control/reload.request" ]; then
    rm -f "$base/control/reload.request"
    id=$((id + 1))
    token=$(head -c 8 /dev/urandom | od -An -tx1 | tr -d ' \n')
    phase=1
  fi
  case $phase in
    1) emit "config INFO reload#$id started"; phase=2 ;;
    2) phase=3 ;;
    3) emit "config INFO reload#$id reading /etc/service/service.yaml"; phase=4 ;;
    4) emit "config WARN reload#$id 2 deprecated keys ignored"; phase=5 ;;
    5) phase=6 ;;
    6) emit "config INFO reload#$id finished token=$token"; phase=0 ;;
  esac
  sleep 0.3
done
SVC
cat > "$base/bin/request-reload" <<'REQ'
#!/bin/sh
# Asks the service to reload its configuration. The service picks the request
# up within a second and reports in its log.
touch /opt/course/follow-live-log/control/reload.request && echo "reload requested"
REQ
chmod 755 "$base/sbin/follow-live-log-svc" "$base/bin/request-reload"

# The history: 120 batches and three reloads, timestamps of the last hour.
start=$(( $(date +%s) - 3600 ))
seq=0
hist() { printf '%s %s\n' "$(date -u -d "@$((start + seq * 25))" +%Y-%m-%dT%H:%M:%SZ)" "$*"; }
{
  for id in 1 2 3; do
    for _ in $(seq 40); do
      seq=$((seq + 1))
      hist "worker[$((seq % 4))] INFO processed batch seq=$seq"
      if [ $((seq % 9)) -eq 0 ]; then hist "scheduler INFO next reload check in 30s"; fi
      case $((seq % 40)) in
        11) hist "config INFO reload#$id started" ;;
        13) hist "config INFO reload#$id reading /etc/service/service.yaml" ;;
        14) hist "config WARN reload#$id 2 deprecated keys ignored" ;;
        16) hist "config INFO reload#$id finished token=$(printf '%s' "history-$id" | sha256sum | cut -c1-16)" ;;
      esac
    done
  done
} > "$base/log/service.log"

wc -l < "$base/log/service.log" > "$base/.history-lines"
sha256sum < "$base/log/service.log" | cut -d' ' -f1 > "$base/.history-sum"

# The service and its log are root's; the learner owns the result directory
# and may drop requests into control/.
chown candidate:candidate "$base" "$base/control"

kd-fixture start svc -- "$base/sbin/follow-live-log-svc" 3 120
