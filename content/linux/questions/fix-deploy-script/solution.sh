#!/usr/bin/env bash
set -euo pipefail
cat > /opt/course/fix-deploy-script/deploy.sh <<'SCRIPT'
#!/usr/bin/env bash
# deploy.sh <release-dir> <target-dir>
#
# Copies the files listed in <release-dir>/manifest.txt (one name per line)
# to <target-dir> and runs <release-dir>/healthcheck.sh <target-dir>.
#
# Exit codes: 0 deployed, 2 usage, 3 no manifest,
#             otherwise the exit code of the failed health check.

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
while IFS= read -r f; do
  cp "$release/$f" "$target/"
  count=$((count + 1))
done < "$release/manifest.txt"

"$release/healthcheck.sh" "$target" | tee "$target/healthcheck.log"
rc=${PIPESTATUS[0]}
if [ "$rc" -eq 0 ]; then
  echo "deployed $count files"
else
  echo "deploy: health check failed" >&2
  exit "$rc"
fi
SCRIPT
