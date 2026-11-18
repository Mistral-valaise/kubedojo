#!/usr/bin/env bash
# vim -r reads the swap file of limits.conf into the buffer, :wq writes it.
# The swap files are removed by hand: Vim never does that after a recovery.
set -euo pipefail
cd /opt/course/vim-recover-swap
vim -es -N -u NONE -r limits.conf -c 'wq' </dev/null
rm -f .limits.conf.swp .upstream.conf.swp
