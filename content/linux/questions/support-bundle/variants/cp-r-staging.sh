#!/usr/bin/env bash
# expect: fail intact
# Stages with plain cp -r: the copies are created with modes cut down by the
# umask, so the logs arrive as 640 instead of 660.
set -euo pipefail
cd /opt/course/support-bundle
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
mkdir -p "$stage/app/log" "$stage/app/bin"
cp -r app/etc "$stage/app/"
rm "$stage/app/etc/tls/server.key" "$stage/app/etc/db.env"
cp app/log/app.log app/log/app.log.1 "$stage/app/log/"
cp app/bin/run.sh "$stage/app/bin/"
sed -i 's/^db_password *=.*/db_password = REDACTED/' "$stage/app/etc/app.conf"
tar -czf bundle.tar.gz -C "$stage" app
