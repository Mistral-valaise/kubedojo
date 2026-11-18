#!/usr/bin/env bash
# expect: fail intact
# dd instead of yy: the on_failure line is cut out of [test] and pasted into
# [deploy]. deploy is right, test lost a setting.
set -euo pipefail
vim -es -N -u NONE \
  -c 'g/^\[legacy-upload\]/.,/^$/d' \
  -c '$put _' \
  -c 'g/^\[lint\]/.,/^$/m$' \
  -c 'g/^\[build\]/.,/^$/m$' \
  -c 'g/^\[test\]/.,/^$/m$' \
  -c 'g/^\[package\]/.,/^$/m$' \
  -c 'g/^\[deploy\]/.,/^$/m$' \
  -c '$d' \
  -c 'g/^on_failure = notify-team$/m$' \
  -c 'wq' /opt/course/vim-reorder-stages/pipeline.conf
