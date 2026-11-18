#!/usr/bin/env bash
# 180 near-identical server lines. Three of them carry a mistyped health-check
# port, each typo different, so one literal search does not find them all.
set -euo pipefail

base=/opt/course/vim-search-fix
rm -rf "$base"
mkdir -p "$base/.grade"

{
  echo '# Backend pool of the shop. Every server is health-checked on the exporter port 9100.'
  echo 'backend shop'
  echo '    balance leastconn'
  echo '    option httpchk GET /healthz'
  for i in $(seq 1 180); do
    port=9100
    case $i in 47) port=9010 ;; 112) port=9001 ;; 163) port=910 ;; esac
    printf '    server web-%03d 10.20.%d.%d:8080 check port %d weight %d\n' \
      "$i" $(( (i - 1) / 60 + 1 )) $(( (i - 1) % 60 + 10 )) "$port" $(( i % 3 * 5 + 10 ))
  done
} > "$base/backend.cfg"

# The corrected file, readable by root only.
sed -E 's/check port [0-9]+/check port 9100/' "$base/backend.cfg" > "$base/.grade/expected"
chmod 700 "$base/.grade"
chown candidate:candidate "$base" "$base/backend.cfg"
