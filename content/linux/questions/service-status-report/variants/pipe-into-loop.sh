#!/usr/bin/env bash
# expect: fail exit-code
# The report is right, but the loop is fed by a pipe: it runs in a subshell,
# the variable set there is lost, and the script always exits with 0.
set -euo pipefail
cat > /opt/course/service-status-report/check-services.sh <<'SCRIPT'
#!/usr/bin/env bash
if [ $# -ne 2 ]; then echo "usage: check-services.sh <list-file> <report-file>" >&2; exit 2; fi
if [ ! -r "$1" ]; then echo "check-services: cannot read $1" >&2; exit 2; fi
failed=0
grep -v '^#' "$1" | while read -r name host port; do
  [ -n "$name" ] || continue
  if nc -z -w 2 "$host" "$port" 2>/dev/null; then
    echo "$name UP"
  else
    echo "$name DOWN"
    failed=1
  fi
done > "$2"
exit "$failed"
SCRIPT
chmod +x /opt/course/service-status-report/check-services.sh
