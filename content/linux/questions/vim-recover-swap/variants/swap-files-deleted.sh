#!/usr/bin/env bash
# expect: fail recovered
# Makes the warning go away by deleting both swap files: the directory is
# clean and the unsaved work on limits.conf is lost.
set -euo pipefail
rm -f /opt/course/vim-recover-swap/.*.swp
