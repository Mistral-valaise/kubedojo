#!/usr/bin/env bash
# expect: pass
# Another correct way: find selects by name (hidden files included, README.md
# excluded) and hands the paths to mv as ./name, which no option parser can
# misread.
set -euo pipefail
cd /opt/course/sort-project-files/project
mkdir src tests docs assets
mkdir assets/img
find . -maxdepth 1 -type f -name 'test_*.py' -exec mv -t tests {} +
find . -maxdepth 1 -type f -name '*.py' -exec mv -t src {} +
find . -maxdepth 1 -type f -name '*.md' ! -name README.md -exec mv -t docs {} +
find . -maxdepth 1 -type f -name '*.png' -exec mv -t assets/img {} +
