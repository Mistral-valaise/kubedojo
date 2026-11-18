#!/usr/bin/env bash
# expect: fail docs top-level
# Everything right except for the hidden Markdown file: *.md does not match
# .todo.md, which stays at the top.
set -euo pipefail
cd /opt/course/sort-project-files/project
mkdir -p src tests docs assets/img
mv -- test_*.py tests/
mv -- *.py src/
mv -- *.md docs/
mv docs/README.md .
mv -- *.png assets/img/
