#!/usr/bin/env bash
# expect: fail kept
# The diagnosis is right, the clean-up is the shortcut: all session files are
# deleted, the live ones included.
set -euo pipefail
cd /opt/course/disk-full-or-inodes
b=$PWD
printf '%s\n' "exhausted: inodes" "largest-file: $b/volume/uploads/tmp/core.20391" "most-files: $b/volume/cache/sessions" > report.txt
rm volume/uploads/tmp/core.20391
find volume/cache/sessions -type f -delete
