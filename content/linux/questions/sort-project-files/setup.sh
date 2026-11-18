#!/usr/bin/env bash
# A flat project directory that has to become src/, tests/, docs/ and
# assets/img/. Some names contain spaces, one starts with a dash and one
# Markdown file is hidden: the three things a careless glob trips over.
set -euo pipefail

base=/opt/course/sort-project-files
rm -rf "$base"
mkdir -p "$base/project"
cd "$base/project"

# Every file gets its own content, so that the guard can tell them apart.
for f in main.py utils.py test_main.py test_utils.py \
         README.md CHANGELOG.md "design notes.md" -draft.md .todo.md \
         logo.png "screenshot 1.png" "screenshot 2.png" \
         Makefile .gitignore .env.example; do
  printf 'content of %s\n' "$f" > "./$f"
done

# What the guard compares: the content of every file, wherever it is now.
find . -type f -exec sha256sum {} + | cut -d' ' -f1 | LC_ALL=C sort > "$base/.contents"

chown -R candidate:candidate "$base"
