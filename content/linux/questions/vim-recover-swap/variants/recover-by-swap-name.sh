#!/usr/bin/env bash
# expect: pass
# Names the swap file instead of the file, writes the recovered text to a
# second file first and compares before replacing the original; the swap
# files are deleted with find.
set -euo pipefail
cd /opt/course/vim-recover-swap
vim -es -N -u NONE -r .limits.conf.swp -c 'w! limits.recovered' -c 'q!' </dev/null
grep -q '^\[checkout\]$' limits.recovered
cat limits.recovered > limits.conf
rm limits.recovered
find . -maxdepth 1 -name '.*.swp' -delete
