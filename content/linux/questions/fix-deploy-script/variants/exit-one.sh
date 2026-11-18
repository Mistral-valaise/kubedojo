#!/usr/bin/env bash
# expect: fail failed-health-check
# Repairs quoting and the argument test, and now fails when the health check
# fails, but still reads $? after the pipe (with pipefail) and exits with a
# fixed 1 instead of the exit code of the health check.
set -euo pipefail
cat > /opt/course/fix-deploy-script/deploy.sh <<'SCRIPT'
#!/usr/bin/env bash
set -o pipefail
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
if "$release/healthcheck.sh" "$target" | tee "$target/healthcheck.log"; then
  echo "deployed $count files"
else
  echo "deploy: health check failed" >&2
  exit 1
fi
SCRIPT
