#!/usr/bin/env bash
# expect: fail nothing-else
# Replaces 100 wherever it stands: the setting is right, but the comment,
# shared_buffers and the log threshold changed with it.
set -euo pipefail
vim -es -N -u NONE -c '%s/100/250/g' -c 'wq' /opt/course/vim-first-edit/db.conf
