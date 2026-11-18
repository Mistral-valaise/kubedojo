#!/usr/bin/env bash
# expect: fail legacy-gone
# Sorts the five stages and copies the line, but never deletes the obsolete
# stage: it stays in front of the others.
set -euo pipefail
vim -es -N -u NONE \
  -c '$put _' \
  -c 'g/^\[lint\]/.,/^$/m$' \
  -c 'g/^\[build\]/.,/^$/m$' \
  -c 'g/^\[test\]/.,/^$/m$' \
  -c 'g/^\[package\]/.,/^$/m$' \
  -c 'g/^\[deploy\]/.,/^$/m$' \
  -c '$d' \
  -c 'g/^on_failure = notify-team$/t$' \
  -c 'wq' /opt/course/vim-reorder-stages/pipeline.conf
