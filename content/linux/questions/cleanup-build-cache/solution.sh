#!/usr/bin/env bash
set -euo pipefail
cd /opt/course/cleanup-build-cache/workspace
find . -type f -name '*.tmp' -delete
rm -- build/obj/*.o
rm -r build/cache
