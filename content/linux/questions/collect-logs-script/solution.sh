#!/usr/bin/env bash
set -euo pipefail
cat > /opt/course/collect-logs-script/collect-logs.sh <<'SCRIPT'
#!/usr/bin/env bash
# collect-logs.sh <source-dir> <dest-dir>
# Copies the non-empty *.log files of <source-dir> to <dest-dir>.
# Exit codes: 0 copied, 1 nothing to copy, 2 usage, 3 source is not a directory.

if [ $# -ne 2 ]; then
  echo "usage: collect-logs.sh <source-dir> <dest-dir>" >&2
  exit 2
fi

src=$1
dst=$2

if [ ! -d "$src" ]; then
  echo "collect-logs: $src: not a directory" >&2
  exit 3
fi

mkdir -p "$dst" || exit 1

n=0
for f in "$src"/*.log; do
  # Regular and not empty. Also false for the literal *.log of a pattern without match.
  if [ -f "$f" ] && [ -s "$f" ]; then
    cp "$f" "$dst"/ && n=$((n + 1))
  fi
done

echo "collected $n"
[ "$n" -gt 0 ]
SCRIPT
chmod +x /opt/course/collect-logs-script/collect-logs.sh
