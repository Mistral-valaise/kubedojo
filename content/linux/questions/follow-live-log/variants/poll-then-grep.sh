#!/usr/bin/env bash
# expect: pass
# Another correct way, close to what a learner does by hand: trigger, look
# again until the finished line of the newest reload is there, then collect
# its lines with grep and cut the token out with awk.
set -eu
b=/opt/course/follow-live-log
last=$(grep -c ' reload#[0-9]* started$' "$b/log/service.log")
"$b/bin/request-reload" >/dev/null
id=$((last + 1))
until grep -q " reload#$id finished " "$b/log/service.log"; do sleep 0.2; done
grep " reload#$id " "$b/log/service.log" > "$b/reload.log"
awk -F'token=' '/ finished / {print $2}' "$b/reload.log" > "$b/token.txt"
