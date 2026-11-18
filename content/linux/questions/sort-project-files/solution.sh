#!/usr/bin/env bash
set -euo pipefail
cd /opt/course/sort-project-files/project
mkdir -p src tests docs assets/img
mv -- test_*.py tests/
mv -- *.py src/
mv -- *.md .*.md docs/
mv docs/README.md .
mv -- *.png assets/img/
