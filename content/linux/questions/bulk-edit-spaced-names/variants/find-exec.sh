#!/usr/bin/env bash
# expect: pass
# Another correct way: find runs sed itself, and the dot is matched with a
# bracket expression. No xargs, so no word splitting to worry about.
set -euo pipefail
cd /opt/course/bulk-edit-spaced-names
find sites -type f -name '*.conf' -exec sed -i 's/db-old[.]internal/db-new.internal/g' {} +
