#!/usr/bin/env bash
# expect: pass
# Another correct script: strict mode, a case for the arguments, and find
# with NUL-separated names read by a while loop from a process substitution
# (a pipe into the loop would lose the counter in a subshell).
set -euo pipefail
cat > /opt/course/collect-logs-script/collect-logs.sh <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

usage() { echo "usage: $(basename "$0") <source-dir> <dest-dir>" >&2; exit 2; }
case $# in
  2) ;;
  *) usage ;;
esac
[[ -d $1 ]] || { echo "collect-logs: $1: not a directory" >&2; exit 3; }

mkdir -p -- "$2"
count=0
while IFS= read -r -d '' file; do
  cp -- "$file" "$2/"
  count=$((count + 1))
done < <(find "$1" -maxdepth 1 -type f -name '*.log' -size +0 -print0)

printf 'collected %d\n' "$count"
if (( count == 0 )); then exit 1; fi
SCRIPT
chmod 0755 /opt/course/collect-logs-script/collect-logs.sh
