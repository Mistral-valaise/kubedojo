#!/usr/bin/env bash
# expect: fail tmp-gone
# Uses ** without dotglob: the hidden file src/.swap.tmp and the file in the
# hidden directory .config are not matched and stay.
set -euo pipefail
cd /opt/course/cleanup-build-cache/workspace
shopt -s globstar
for f in **/*.tmp; do
  if [ -f "$f" ]; then rm -- "$f"; fi
done
rm -- build/obj/*.o
rm -r build/cache
