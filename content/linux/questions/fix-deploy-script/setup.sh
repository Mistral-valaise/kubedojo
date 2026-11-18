#!/usr/bin/env bash
# A deploy script with the classic shell bugs: a test that is a redirection,
# unquoted variables, a for loop over $(cat ...), and an exit code swallowed
# by a pipe. It works for simple names and a passing health check only.
set -euo pipefail

base=/opt/course/fix-deploy-script
rm -rf "$base"
mkdir -p "$base/sample/release 1.4"

cat > "$base/deploy.sh" <<'SCRIPT'
#!/usr/bin/env bash
# deploy.sh <release-dir> <target-dir>
#
# Copies the files listed in <release-dir>/manifest.txt (one name per line)
# to <target-dir> and runs <release-dir>/healthcheck.sh <target-dir>.
#
# Exit codes: 0 deployed, 2 usage, 3 no manifest,
#             otherwise the exit code of the failed health check.

release=$1
target=$2

if [ $# < 2 ]; then
  echo "usage: deploy.sh <release-dir> <target-dir>" >&2
  exit 2
fi

if [ ! -f $release/manifest.txt ]; then
  echo "deploy: no manifest.txt in $release" >&2
  exit 3
fi

mkdir -p $target

count=0
for f in $(cat $release/manifest.txt); do
  cp $release/$f $target/
  count=$((count + 1))
done

$release/healthcheck.sh $target | tee $target/healthcheck.log
if [ $? = 0 ]; then
  echo "deployed $count files"
else
  echo "deploy: health check failed" >&2
fi
SCRIPT
chmod 0755 "$base/deploy.sh"

# A release to try the script with. The health check of this release fails.
cd "$base/sample/release 1.4"
echo '<h1>shop 1.4</h1>'       > index.html
echo 'body { margin: 0 }'      > style.css
echo '1.4: new checkout page'  > "release notes.txt"
printf '%s\n' index.html style.css "release notes.txt" > manifest.txt
cat > healthcheck.sh <<'HC'
#!/usr/bin/env bash
# healthcheck.sh <target-dir>: exit 0 when the deployed site is complete.
echo "health check of: $1"
if [ -f "$1/index.html" ] && [ -f "$1/version.txt" ]; then
  echo "result: healthy"
  exit 0
fi
echo "result: version.txt is missing"
exit 4
HC
chmod 0755 healthcheck.sh

chown -R candidate:candidate "$base"
