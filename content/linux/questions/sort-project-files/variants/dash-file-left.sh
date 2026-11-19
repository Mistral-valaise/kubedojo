#!/usr/bin/env bash
# expect: fail docs top-level
# Without --, mv takes -draft.md for options and moves nothing. The learner
# then moves the other Markdown files by name and overlooks the one that
# caused the error.
cd /opt/course/sort-project-files/project
mkdir -p src tests docs assets/img
mv test_*.py tests/
mv *.py src/
mv *.md docs/ 2>/dev/null
mv CHANGELOG.md "design notes.md" .todo.md docs/
mv *.png assets/img/
