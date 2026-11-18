#!/usr/bin/env bash
# expect: pass
# No editor: awk reads the file paragraph by paragraph (RS empty) and prints
# the stages in the wanted order.
set -euo pipefail
f=/opt/course/vim-reorder-stages/pipeline.conf
awk -v RS= -v want='lint build test package deploy' '
  NR == 1 { head = $0; next }
  { name = $0; sub(/\n.*/, "", name); block[name] = $0 }
  END {
    print head
    n = split(want, w, " ")
    for (i = 1; i <= n; i++) {
      b = block["[" w[i] "]"]
      if (w[i] == "deploy") b = b "\non_failure = notify-team"
      printf "\n%s\n", b
    }
  }' "$f" > "$f.new"
mv "$f.new" "$f"
