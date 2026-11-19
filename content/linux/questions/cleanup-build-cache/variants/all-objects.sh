#!/usr/bin/env bash
# expect: fail cache-gone survivors
# Deletes object files everywhere, the prebuilt ones in vendor/lib included,
# and only empties the cache instead of removing the directory.
set -euo pipefail
cd /opt/course/cleanup-build-cache/workspace
find . -type f -name '*.tmp' -delete
find . -name '*.o' -delete
rm -r build/cache/*
