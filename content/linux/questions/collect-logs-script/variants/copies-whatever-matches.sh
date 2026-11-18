#!/usr/bin/env bash
# expect: fail copies no-matches
# Quotes correctly but never tests what the pattern matched: the empty file is
# copied and counted, in both runs.
set -euo pipefail
cat > /opt/course/collect-logs-script/collect-logs.sh <<'SCRIPT'
#!/usr/bin/env bash
if [ $# -ne 2 ]; then echo "usage: collect-logs.sh <source-dir> <dest-dir>" >&2; exit 2; fi
if [ ! -d "$1" ]; then echo "collect-logs: $1: not a directory" >&2; exit 3; fi
mkdir -p "$2"
n=0
for f in "$1"/*.log; do
  cp "$f" "$2"/ 2>/dev/null && n=$((n + 1))
done
echo "collected $n"
[ "$n" -gt 0 ]
SCRIPT
chmod +x /opt/course/collect-logs-script/collect-logs.sh
