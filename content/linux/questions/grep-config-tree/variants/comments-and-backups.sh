#!/usr/bin/env bash
# expect: fail weak-tls decom-count no-server-name
# The right regular expressions for the values, but no thought for what is
# active: commented lines count, the .bak file is searched too, and a
# commented server_name passes for a directive.
set -euo pipefail
b=/opt/course/grep-config-tree
grep -rlE 'ssl_protocols.*TLSv1(\.1)?[ ;]' "$b/etc" > "$b/weak-tls.txt"
grep -rhE '(^|[^0-9.])10\.20\.[0-9]+\.[0-9]+' "$b/etc" | wc -l > "$b/decom-count.txt"
grep -L 'server_name' "$b"/etc/nginx/sites/*.conf > "$b/no-server-name.txt"
