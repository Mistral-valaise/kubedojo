#!/usr/bin/env bash
# expect: fail no-secrets
# One tar command with the right excludes, straight from the live files: the
# secret files are left out, but the password inside app.conf is shipped.
set -euo pipefail
cd /opt/course/support-bundle
tar -czf bundle.tar.gz --exclude='*.key' --exclude='*.env' \
  app/etc app/log/app.log app/log/app.log.1 app/bin/run.sh
