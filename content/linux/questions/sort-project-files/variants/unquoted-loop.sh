#!/usr/bin/env bash
# expect: fail docs assets top-level
# Loops over the output of ls without quotes: names with spaces are split
# into words that do not exist, so those files stay where they were.
cd /opt/course/sort-project-files/project
mkdir -p src tests docs assets/img
mv -- test_*.py tests/
mv -- *.py src/
for f in $(ls -A | grep '\.md$' | grep -v '^README'); do mv -- $f docs/ 2>/dev/null; done
for f in $(ls | grep '\.png$'); do mv -- $f assets/img/ 2>/dev/null; done
exit 0
