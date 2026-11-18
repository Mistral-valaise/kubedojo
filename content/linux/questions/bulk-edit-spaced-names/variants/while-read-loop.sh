#!/usr/bin/env bash
# expect: pass
# Another correct way: a loop that reads NUL-separated names, edits a copy and
# moves it into place.
set -euo pipefail
find /opt/course/bulk-edit-spaced-names/sites -type f -name '*.conf' -print0 |
  while IFS= read -r -d '' f; do
    sed 's/db-old\.internal/db-new.internal/g' "$f" > "$f.tmp"
    mv "$f.tmp" "$f"
  done
