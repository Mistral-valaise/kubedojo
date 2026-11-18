#!/usr/bin/env bash
# expect: pass
# Another correct way: awk finds the drift by comparing key/value maps, and
# web-02 is rebuilt from the reference with its own two host settings put back.
set -euo pipefail
cd /opt/course/config-drift
awk -F'[ \t]*=[ \t]*' '
  /^#/ || NF < 2 { next }
  { sub(/[ \t]+$/, "", $2) }
  FNR == NR { ref[$1] = $2; next }
  { seen[$1] = 1; if (!($1 in ref) || ref[$1] != $2) print $1 }
  END { for (k in ref) if (!(k in seen)) print k }
' web-01/app.conf web-02/app.conf | grep -vxE 'hostname|listen_address' | sort > drift.txt

host=$(awk -F'[ \t]*=[ \t]*' '$1 == "hostname" {print $2}' web-02/app.conf)
addr=$(awk -F'[ \t]*=[ \t]*' '$1 == "listen_address" {print $2}' web-02/app.conf)
sed -e "s/^hostname = .*/hostname = $host/" -e "s/^listen_address = .*/listen_address = $addr/" \
  web-01/app.conf > web-02/app.conf.new
mv web-02/app.conf.new web-02/app.conf
