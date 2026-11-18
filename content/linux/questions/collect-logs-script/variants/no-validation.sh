#!/usr/bin/env bash
# expect: fail usage not-a-directory
# The copy loop is right, but the script trusts its arguments: no usage
# message, and a missing source ends as "collected 0" with exit code 1.
set -euo pipefail
cat > /opt/course/collect-logs-script/collect-logs.sh <<'SCRIPT'
#!/usr/bin/env bash
src=$1
dst=$2
mkdir -p "$dst" 2>/dev/null
n=0
for f in "$src"/*.log; do
  if [ -f "$f" ] && [ -s "$f" ]; then
    cp "$f" "$dst"/ && n=$((n + 1))
  fi
done
echo "collected $n"
[ "$n" -gt 0 ]
SCRIPT
chmod +x /opt/course/collect-logs-script/collect-logs.sh
