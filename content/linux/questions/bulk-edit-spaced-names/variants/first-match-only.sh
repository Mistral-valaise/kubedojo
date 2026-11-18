#!/usr/bin/env bash
# expect: fail no-old-host only-the-host
# Safe file handling, but s/// without the g flag: the second host in the
# line of checkout.conf keeps the old name.
set -euo pipefail
find /opt/course/bulk-edit-spaced-names/sites -type f -name '*.conf' -print0 \
  | xargs -0 sed -i 's/db-old\.internal/db-new.internal/'
