#!/usr/bin/env bash
# expect: fail keys comments other-values
# The first idea: replace warn everywhere. The seven values are right, but
# warning became warninging, three settings were renamed, comments changed.
set -euo pipefail
vim -es -N -u NONE -c '%s/warn/warning/g' -c 'wq' /opt/course/vim-replace-level/logging.conf
