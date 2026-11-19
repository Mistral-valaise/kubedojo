#!/usr/bin/env bash
set -euo pipefail
b=/opt/course/grep-config-tree
grep -rlE --include='*.conf' '^[[:space:]]*ssl_protocols.*TLSv1(\.1)?[ ;]' "$b/etc" > "$b/weak-tls.txt"
grep -rhE --include='*.conf' '^[^#]*(^|[^0-9.])10\.20\.[0-9]+\.[0-9]+' "$b/etc" | wc -l > "$b/decom-count.txt"
grep -LE '^[[:space:]]*server_name' "$b"/etc/nginx/sites/*.conf > "$b/no-server-name.txt"
