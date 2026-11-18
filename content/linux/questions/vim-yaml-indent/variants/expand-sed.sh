#!/usr/bin/env bash
# expect: pass
# Shell tools instead of an editor: expand converts the tabs, sed adds four
# spaces to one block and removes four from the other.
set -euo pipefail
f=/opt/course/vim-yaml-indent/pod.yaml
expand -t 2 "$f" \
  | sed -E '/^  env:/,/^      memory:/s/^/    /; /^      volumes:/,$s/^    //' > "$f.new"
mv "$f.new" "$f"
