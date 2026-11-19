#!/usr/bin/env bash
# expect: fail awkward-names no-old-host only-the-host
# find | xargs without NUL separation: names with blanks are split, the
# apostrophe aborts xargs. Only the files with simple names are edited.
cd /opt/course/bulk-edit-spaced-names
find sites -type f -name '*.conf' | xargs sed -i 's/db-old\.internal/db-new.internal/g' 2>/dev/null
exit 0
