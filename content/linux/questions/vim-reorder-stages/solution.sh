#!/usr/bin/env bash
# Ex commands instead of keystrokes: ":range d" deletes, ":range m$" moves a
# block to the end of the file, ":t$" copies a line there. Moving the stages
# to the end one after the other, in the wanted order, sorts them.
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
  -c 'g/^on_failure = notify-team$/t$' \
  -c 'wq' /opt/course/vim-reorder-stages/pipeline.conf
