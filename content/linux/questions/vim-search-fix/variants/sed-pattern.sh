#!/usr/bin/env bash
# expect: pass
# The same idea with sed: replace whatever number follows "check port".
set -euo pipefail
sed -i -E '/^ *server /s/(check port )[0-9]+/\19100/' /opt/course/vim-search-fix/backend.cfg
