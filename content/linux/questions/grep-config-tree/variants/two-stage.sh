#!/usr/bin/env bash
# expect: pass
# Another correct way: find selects the files, a first grep drops comment
# lines, a second one searches what is left. File names come from grep -H
# and are cut off again.
set -euo pipefail
b=/opt/course/grep-config-tree
active() { find "$b/etc" -type f -name '*.conf' -exec grep -Hv '^[[:space:]]*#' {} +; }
active | grep -E ':[[:space:]]*ssl_protocols ' | grep -E 'TLSv1(\.1)?( |;)' | cut -d: -f1 | sort -u > "$b/weak-tls.txt"
active | cut -d: -f2- | grep -cE '\<10\.20\.[0-9]{1,3}\.[0-9]{1,3}\>' > "$b/decom-count.txt"
for f in "$b"/etc/nginx/sites/*.conf; do
  grep -v '^[[:space:]]*#' "$f" | grep -q 'server_name' || echo "$f"
done > "$b/no-server-name.txt"
