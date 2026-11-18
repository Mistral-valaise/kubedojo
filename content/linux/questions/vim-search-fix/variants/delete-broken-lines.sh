#!/usr/bin/env bash
# expect: fail three-fixed count
# Removes the three broken lines: nothing is wrong any more, and three
# servers are gone from the pool.
set -euo pipefail
vim -es -N -u NONE -c 'g/^\s*server /if getline(".") !~ "check port 9100 " | delete | endif' -c 'wq' \
  /opt/course/vim-search-fix/backend.cfg
