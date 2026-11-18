#!/usr/bin/env bash
# expect: fail blanks-in-names
# Quotes every variable and repairs the test and the exit code, but keeps
# `for f in $(cat ...)`: file names with blanks in the manifest are still split.
set -euo pipefail
cat > /opt/course/fix-deploy-script/deploy.sh <<'SCRIPT'
#!/usr/bin/env bash
if [ $# -lt 2 ]; then
  echo "usage: deploy.sh <release-dir> <target-dir>" >&2
  exit 2
fi
release=$1
target=$2
if [ ! -f "$release/manifest.txt" ]; then
  echo "deploy: no manifest.txt in $release" >&2
  exit 3
fi
mkdir -p "$target"
count=0
for f in $(cat "$release/manifest.txt"); do
  cp "$release/$f" "$target/" 2>/dev/null
  count=$((count + 1))
done
"$release/healthcheck.sh" "$target" > "$target/healthcheck.log"
rc=$?
if [ "$rc" -ne 0 ]; then
  echo "deploy: health check failed" >&2
  exit "$rc"
fi
echo "deployed $count files"
SCRIPT
