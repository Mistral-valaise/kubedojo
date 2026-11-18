#!/usr/bin/env bash
set -euo pipefail
find /opt/course/bulk-edit-spaced-names/sites -type f -name '*.conf' -print0 \
  | xargs -0 sed -i 's/db-old\.internal/db-new.internal/g'
