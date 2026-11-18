#!/usr/bin/env bash
# :retab converts the tabs with tabstop=2, ":range>>" and ":range<<" shift a
# block by two shiftwidths: what V plus > . and < . do on screen.
set -euo pipefail
vim -es -N -u NONE \
  -c 'set tabstop=2 shiftwidth=2 expandtab' \
  -c 'retab' \
  -c '/^  env:/,/^      memory:/>>' \
  -c '/^      volumes:/,$<<' \
  -c 'wq' /opt/course/vim-yaml-indent/pod.yaml
