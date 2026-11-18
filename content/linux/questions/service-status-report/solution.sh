#!/usr/bin/env bash
set -euo pipefail
cat > /opt/course/service-status-report/check-services.sh <<'SCRIPT'
#!/usr/bin/env bash
# check-services.sh <list-file> <report-file>
# Exit codes: 0 all services UP, 1 at least one DOWN, 2 wrong call.

if [ $# -ne 2 ]; then
  echo "usage: check-services.sh <list-file> <report-file>" >&2
  exit 2
fi

list=$1
report=$2

if [ ! -f "$list" ] || [ ! -r "$list" ]; then
  echo "check-services: cannot read $list" >&2
  exit 2
fi

# is_up <host> <port>: true when a TCP connection can be opened.
is_up() {
  nc -z -w 2 "$1" "$2" 2>/dev/null
}

failed=0
while read -r name host port || [ -n "$name" ]; do
  case $name in
    '' | '#'*) continue ;;
  esac
  if is_up "$host" "$port"; then
    echo "$name UP"
  else
    echo "$name DOWN"
    failed=1
  fi
done < "$list" > "$report"

exit "$failed"
SCRIPT
chmod +x /opt/course/service-status-report/check-services.sh
