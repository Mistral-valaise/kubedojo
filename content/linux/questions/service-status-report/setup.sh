#!/usr/bin/env bash
# Two local listeners (ports 21080 and 21443) stand in for healthy services;
# nothing listens on 21090 and 21091. The learner writes check-services.sh,
# and the checks run it against lists of their own that use these ports.
set -euo pipefail
kd-fixture reset

base=/opt/course/service-status-report
rm -rf "$base"
mkdir -p "$base/www"
echo ok > "$base/www/index.html"

kd-fixture start web   -- python3 -m http.server 21080 --bind 127.0.0.1 --directory "$base/www"
kd-fixture start admin -- python3 -m http.server 21443 --bind 127.0.0.1 --directory "$base/www"

cat > "$base/services.txt" <<'LIST'
# name host port
# checked by the on-call engineer after every deployment

shop-web      127.0.0.1 21080
shop-admin    127.0.0.1 21443

# moved to the new cluster last week
shop-search   127.0.0.1 21090
LIST

chown candidate:candidate "$base" "$base/services.txt"
