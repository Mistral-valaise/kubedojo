#!/usr/bin/env bash
# expect: pass
# Another correct repair: strict mode, an arithmetic test, mapfile instead of
# a read loop, and a redirection instead of tee so that no pipe is involved.
set -euo pipefail
cat > /opt/course/fix-deploy-script/deploy.sh <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

if (( $# < 2 )); then
  echo "usage: deploy.sh <release-dir> <target-dir>" >&2
  exit 2
fi
release=$1
target=$2
[[ -f $release/manifest.txt ]] || { echo "deploy: no manifest.txt in $release" >&2; exit 3; }

mkdir -p -- "$target"
mapfile -t files < "$release/manifest.txt"
for f in "${files[@]}"; do
  cp -- "$release/$f" "$target/"
done

rc=0
"$release/healthcheck.sh" "$target" > "$target/healthcheck.log" 2>&1 || rc=$?
if (( rc != 0 )); then
  echo "deploy: health check failed with exit code $rc" >&2
  exit "$rc"
fi
echo "deployed ${#files[@]} files"
SCRIPT
