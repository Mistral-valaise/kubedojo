#!/usr/bin/env bash
# An access log in the combined format, generated from a fixed table so that
# every count is known. Traps for text matching instead of field matching: a
# path that contains "500", and successful responses of 500 to 504 bytes.
set -euo pipefail

base=/opt/course/access-log-report
rm -rf "$base"
mkdir -p "$base"

# path status requests
awk '
  { for (i = 0; i < $3; i++) { n++; path[n] = $1; status[n] = $2 } }
  END {
    # A fixed permutation: 7919 is prime and does not divide n, so every line gets its own slot.
    for (i = 1; i <= n; i++) slot[(i * 7919) % n] = i
    for (s = 0; s < n; s++) {
      i = slot[s]
      if (status[i] == 200)      bytes = (path[i] == "/img/500.png") ? 502 : 500 + (i * 37) % 900
      else if (status[i] == 304) bytes = 0
      else                       bytes = 150 + (i * 13) % 60
      t = 8 * 3600 + s * 11
      printf "10.%d.%d.%d - - [14/Mar/2025:%02d:%02d:%02d +0000] \"%s %s HTTP/1.1\" %d %d \"-\" \"%s\"\n",
        20 + s % 3, (s * 7) % 5, 10 + (s * 31) % 40, t / 3600, (t % 3600) / 60, t % 60,
        (path[i] ~ /checkout|cart|login/ && status[i] != 302 && i % 2) ? "POST" : "GET",
        path[i], status[i], bytes, (s % 4) ? "Mozilla/5.0 (X11; Linux x86_64)" : "curl/8.9.1"
    }
  }' > "$base/access.log" <<'TABLE'
/ 200 50
/api/checkout 200 40
/api/checkout 502 9
/api/checkout 500 5
/api/cart 200 30
/api/cart 500 11
/api/search 200 25
/api/search 503 7
/login 200 20
/login 302 6
/login 500 4
/api/orders 504 2
/api/orders 499 3
/healthz 200 18
/healthz 500 1
/img/500.png 200 12
/static/app.js 200 15
/static/app.js 304 10
/favicon.ico 404 8
/admin 403 3
TABLE

sha256sum "$base/access.log" | cut -d' ' -f1 > "$base/.manifest"

# The learner writes the reports into the directory; the log is root's.
chown candidate:candidate "$base"
