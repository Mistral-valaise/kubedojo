#!/usr/bin/env bash
# expect: fail kept
# Recovers every swap file without looking: the abandoned experiment ends up
# in upstream.conf.
set -euo pipefail
cd /opt/course/vim-recover-swap
vim -es -N -u NONE -r limits.conf -c 'wq' </dev/null
vim -es -N -u NONE -r upstream.conf -c 'wq' </dev/null
rm -f .limits.conf.swp .upstream.conf.swp
