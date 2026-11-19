#!/usr/bin/env bash
set -euo pipefail
cd /opt/course/support-bundle

stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT

# Copy the requested files with their modes: one tar writes, one unpacks.
# Naming the directory app/etc takes hidden files along; the patterns leave
# the key and the env file out.
tar -cf - --exclude='*.key' --exclude='*.env' \
  app/etc app/log/app.log app/log/app.log.1 app/bin/run.sh | tar -xpf - -C "$stage"

# Redact in the copy. sed -i keeps the mode of the file.
sed -i 's/^db_password *=.*/db_password = REDACTED/' "$stage/app/etc/app.conf"

tar -czf bundle.tar.gz -C "$stage" app
tar -tzvf bundle.tar.gz
