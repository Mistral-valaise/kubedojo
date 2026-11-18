#!/usr/bin/env bash
# expect: pass
# awk, comparing the value as a field instead of matching text: a line with
# exactly "name = warn" gets a new third field.
set -euo pipefail
f=/opt/course/vim-replace-level/logging.conf
awk 'NF == 3 && $2 == "=" && $3 == "warn" { print $1, "=", "warning"; next } { print }' "$f" > "$f.new"
mv "$f.new" "$f"
