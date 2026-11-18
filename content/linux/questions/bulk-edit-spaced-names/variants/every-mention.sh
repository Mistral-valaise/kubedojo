#!/usr/bin/env bash
# expect: fail others-untouched
# Selects by content instead of by name: every file that mentions the old
# host is edited, including the backup, the notes and the disabled site.
set -euo pipefail
grep -rlZ 'db-old\.internal' /opt/course/bulk-edit-spaced-names/sites \
  | xargs -0 sed -i 's/db-old\.internal/db-new.internal/g'
