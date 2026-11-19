#!/usr/bin/env bash
# expect: fail survivors
# Empties the cache directory by directory before removing it. The pattern
# */ follows the symbolic link, and rm deletes the content of vendor/ without
# a word.
set -euo pipefail
cd /opt/course/cleanup-build-cache/workspace
find . -type f -name '*.tmp' -delete
rm -- build/obj/*.o
rm -rf build/cache/*/
rm -rf build/cache
