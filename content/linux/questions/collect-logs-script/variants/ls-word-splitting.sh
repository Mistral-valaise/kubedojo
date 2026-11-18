#!/usr/bin/env bash
# expect: fail copies
# Loops over the output of ls: names with blanks are split into words that
# do not exist, and nothing is collected. Arguments are validated correctly.
set -euo pipefail
cat > /opt/course/collect-logs-script/collect-logs.sh <<'SCRIPT'
#!/usr/bin/env bash
if [ $# -ne 2 ]; then echo "usage: collect-logs.sh <source-dir> <dest-dir>" >&2; exit 2; fi
if [ ! -d "$1" ]; then echo "collect-logs: $1: not a directory" >&2; exit 3; fi
mkdir -p "$2"
n=0
for f in $(ls "$1"/*.log 2>/dev/null); do
  if [ -f "$f" ] && [ -s "$f" ]; then
    cp "$f" "$2"/ && n=$((n + 1))
  fi
done
echo "collected $n"
[ "$n" -gt 0 ]
SCRIPT
chmod +x /opt/course/collect-logs-script/collect-logs.sh
