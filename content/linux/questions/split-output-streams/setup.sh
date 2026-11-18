#!/usr/bin/env bash
# A tool that writes records to stdout, complaints to stderr (interleaved) and
# exits with 3. Its output is deterministic, so the checks can run it again.
set -euo pipefail

base=/opt/course/split-output-streams
rm -rf "$base"
mkdir -p "$base/bin"

cat > "$base/bin/inventory-export" <<'TOOL'
#!/usr/bin/env bash
# inventory-export: prints the host inventory as CSV on standard output.
# Records that cannot be exported are reported on standard error.
# Exit codes: 0 all records exported, 3 some records were skipped.
skipped=0
n=0
echo "host,role,cpus,memory_gb"
while IFS=, read -r host role cpus mem; do
  n=$((n + 1))
  if [ -z "$cpus" ] || [ -z "$mem" ]; then
    echo "WARN record $n ($host): incomplete, skipped" >&2
    skipped=$((skipped + 1))
    continue
  fi
  case $cpus in
    *[!0-9]*)
      echo "WARN record $n ($host): cpus is not a number: $cpus, skipped" >&2
      skipped=$((skipped + 1))
      continue ;;
  esac
  echo "$host,$role,$cpus,$mem"
done <<'DATA'
web-01,frontend,4,8
web-02,frontend,4,8
web-03,frontend,,8
api-01,backend,8,16
api-02,backend,eight,16
db-01,database,16,64
db-02,database,16,64
cache-01,cache,2,
queue-01,broker,4,16
warn-relay-01,monitoring,2,4
batch-01,worker,8,32
DATA
if [ "$skipped" -gt 0 ]; then
  echo "inventory-export: $skipped of $n records skipped" >&2
  exit 3
fi
TOOL
chmod 0755 "$base/bin/inventory-export"

sha256sum "$base/bin/inventory-export" | cut -d' ' -f1 > "$base/.manifest"

# The learner writes the result files into the directory; the tool is root's.
chown candidate:candidate "$base"
