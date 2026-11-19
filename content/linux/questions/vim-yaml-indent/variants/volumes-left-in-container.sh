#!/usr/bin/env bash
# expect: fail volumes
# Repairs the pasted block and overlooks the last one: volumes is still a key
# of the container.
set -euo pipefail
vim -es -N -u NONE \
  -c 'set tabstop=2 shiftwidth=2 expandtab' \
  -c 'retab' \
  -c '/^  env:/,/^      memory:/>>' \
  -c 'wq' /opt/course/vim-yaml-indent/pod.yaml
