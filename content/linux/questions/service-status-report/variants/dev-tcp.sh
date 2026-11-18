#!/usr/bin/env bash
# expect: pass
# Another correct script: strict mode, bash's /dev/tcp instead of nc, the
# list filtered by grep through a process substitution (which also supplies
# the final newline), and the report written to a temporary file first.
set -euo pipefail
cat > /opt/course/service-status-report/check-services.sh <<'SCRIPT'
#!/usr/bin/env bash
set -uo pipefail

die() { echo "check-services: $*" >&2; exit 2; }
[[ $# -eq 2 ]] || die "usage: check-services.sh <list-file> <report-file>"
[[ -r $1 && -f $1 ]] || die "cannot read list file: $1"

tmp=$(mktemp) || die "cannot create a temporary file"
trap 'rm -f "$tmp"' EXIT

down=0
while read -r name host port; do
  if timeout 2 bash -c 'exec 3<>"/dev/tcp/$0/$1"' "$host" "$port" 2>/dev/null; then
    state=UP
  else
    state=DOWN
    down=$((down + 1))
  fi
  printf '%s %s\n' "$name" "$state" >> "$tmp"
done < <(grep -Ev '^[[:space:]]*(#|$)' "$1")

cat "$tmp" > "$2"
(( down == 0 ))
SCRIPT
chmod 0755 /opt/course/service-status-report/check-services.sh
