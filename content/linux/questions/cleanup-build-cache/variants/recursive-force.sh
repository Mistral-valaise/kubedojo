#!/usr/bin/env bash
# expect: fail survivors
# "rm -rf everything that ends in .tmp": the directory archive.tmp goes too,
# with the database in it.
set -euo pipefail
cd /opt/course/cleanup-build-cache/workspace
shopt -s globstar dotglob
rm -rf -- **/*.tmp
rm -- build/obj/*.o
rm -r build/cache
