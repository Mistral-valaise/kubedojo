#!/usr/bin/env bash
# A setting has the value warn when its line ends in "= warn".
set -euo pipefail
vim -es -N -u NONE \
  -c '%s/= warn$/= warning/' \
  -c 'wq' /opt/course/vim-replace-level/logging.conf
