#!/usr/bin/env bash
# expect: fail report last-line
# Appends to the report line by line, so a second run doubles it and a stale
# report is kept, and a plain `while read` drops a last line without newline.
set -euo pipefail
cat > /opt/course/service-status-report/check-services.sh <<'SCRIPT'
#!/usr/bin/env bash
if [ $# -ne 2 ]; then echo "usage: check-services.sh <list-file> <report-file>" >&2; exit 2; fi
if [ ! -r "$1" ]; then echo "check-services: cannot read $1" >&2; exit 2; fi
failed=0
while read -r name host port; do
  case $name in ''|'#'*) continue ;; esac
  if nc -z -w 2 "$host" "$port" 2>/dev/null; then
    echo "$name UP" >> "$2"
  else
    echo "$name DOWN" >> "$2"
    failed=1
  fi
done < "$1"
exit "$failed"
SCRIPT
chmod +x /opt/course/service-status-report/check-services.sh
