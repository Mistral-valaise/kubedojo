#!/usr/bin/env bash
# expect: fail no-old-host only-the-host
# A shell pattern instead of find: quoted correctly, so blanks are no problem,
# but * does not match the hidden file blog/.cache.conf.
set -euo pipefail
cd /opt/course/bulk-edit-spaced-names/sites
sed -i 's/db-old\.internal/db-new.internal/g' */*.conf
