#!/usr/bin/env bash
# expect: pass
# Another correct way: short options, an exported variable, an absolute path,
# and the exit status looked up with man instead of typed from memory.
set -uo pipefail
t=/opt/course/read-tool-manual
"$t/bin/relcheck" -a -s size -f tsv "$t/releases" > "$t/report.tsv"
export RELCHECK_IGNORE='*.tmp:*.bak'
"$t/bin/relcheck" -a -s size -f tsv "$t/releases" > "$t/report-clean.tsv"
MANWIDTH=80 man -M "$t/man" relcheck 2>/dev/null | col -b \
  | awk '/^EXIT STATUS/ {s=1; next} s && /NOMANIFEST/ {print $1; exit}' \
  > "$t/no-manifest-status.txt"
