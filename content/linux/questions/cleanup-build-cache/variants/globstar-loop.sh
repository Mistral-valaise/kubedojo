#!/usr/bin/env bash
# expect: pass
# Another correct way without find: ** with globstar and dotglob reaches every
# depth and hidden names, and the loop tests each match before it removes it.
set -euo pipefail
cd /opt/course/cleanup-build-cache/workspace
shopt -s globstar dotglob nullglob
for f in **/*.tmp; do
  if [ -f "$f" ] && [ ! -L "$f" ]; then rm -- "$f"; fi
done
find build/obj -maxdepth 1 -type f -name '*.o' -exec rm {} +
rm -rf build/cache
