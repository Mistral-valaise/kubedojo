#!/usr/bin/env bash
# The ex commands a learner types after ":" in Vim, run without a screen.
set -euo pipefail
vim -es -N -u NONE \
  -c '/^max_connections/' \
  -c 's/100/250/' \
  -c 'wq' /opt/course/vim-first-edit/db.conf
