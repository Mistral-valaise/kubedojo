#!/usr/bin/env bash
# expect: fail all-9100 three-fixed
# Searches for the first typo that was spotted and replaces that number
# everywhere: the two other typos are different numbers and stay.
set -euo pipefail
vim -es -N -u NONE -c '%s/port 9010/port 9100/g' -c 'wq' /opt/course/vim-search-fix/backend.cfg
