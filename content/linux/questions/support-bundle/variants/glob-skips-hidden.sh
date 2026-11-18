#!/usr/bin/env bash
# expect: fail members
# app/etc/* instead of app/etc: the shell pattern does not match hidden
# files, so etc/.features is missing from the bundle.
set -euo pipefail
cd /opt/course/support-bundle
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
tar -cf - --exclude='*.key' --exclude='*.env' \
  app/etc/* app/log/app.log app/log/app.log.1 app/bin/run.sh | tar -xpf - -C "$stage"
sed -i 's/^db_password *=.*/db_password = REDACTED/' "$stage/app/etc/app.conf"
tar -czf bundle.tar.gz -C "$stage" app
