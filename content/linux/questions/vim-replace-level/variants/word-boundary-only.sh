#!/usr/bin/env bash
# expect: fail comments other-values
# Whole words only, but without looking at the matches: setting names are
# safe, the word in the comments and inside "warn: " is replaced as well.
set -euo pipefail
vim -es -N -u NONE -c '%s/\<warn\>/warning/g' -c 'wq' /opt/course/vim-replace-level/logging.conf
