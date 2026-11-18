#!/usr/bin/env bash
# :g runs a command on every line that matches: here a substitute that sets
# the port, whatever number stands there.
set -euo pipefail
vim -es -N -u NONE \
  -c 'g/^\s*server /s/check port \d\+/check port 9100/' \
  -c 'wq' /opt/course/vim-search-fix/backend.cfg
