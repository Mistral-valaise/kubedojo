#!/usr/bin/env bash
# expect: fail decom-count
# Searches for 10.20. as typed: the dots match any character and nothing
# guards the front, so 110.20.1.5, 1012034 and the commented lines are counted.
set -euo pipefail
b=/opt/course/grep-config-tree
grep -rlE --include='*.conf' '^[[:space:]]*ssl_protocols.*TLSv1(\.1)?[ ;]' "$b/etc" > "$b/weak-tls.txt"
grep -rh --include='*.conf' '10.20.' "$b/etc" | wc -l > "$b/decom-count.txt"
grep -LE '^[[:space:]]*server_name' "$b"/etc/nginx/sites/*.conf > "$b/no-server-name.txt"
