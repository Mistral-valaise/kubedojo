#!/usr/bin/env bash
# expect: fail swap-gone
# Recovers and saves, and expects Vim to tidy up: the swap file of
# limits.conf is still there.
set -euo pipefail
cd /opt/course/vim-recover-swap
vim -es -N -u NONE -r limits.conf -c 'wq' </dev/null
rm -f .upstream.conf.swp
