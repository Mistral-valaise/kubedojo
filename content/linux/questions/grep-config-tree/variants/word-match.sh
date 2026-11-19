#!/usr/bin/env bash
# expect: fail weak-tls
# Trusts -w to tell TLSv1 from TLSv1.2. A dot ends a word, so every file that
# mentions any TLS version matches, comments included.
set -euo pipefail
b=/opt/course/grep-config-tree
grep -rlw --include='*.conf' 'TLSv1' "$b/etc" > "$b/weak-tls.txt"
grep -rhE --include='*.conf' '^[^#]*(^|[^0-9.])10\.20\.[0-9]+\.[0-9]+' "$b/etc" | wc -l > "$b/decom-count.txt"
grep -LE '^[[:space:]]*server_name' "$b"/etc/nginx/sites/*.conf > "$b/no-server-name.txt"
