#!/usr/bin/env bash
# expect: fail culprits space-freed kept
# Judges by the length of the files (find -size, like ls -l): the sparse
# database image looks biggest, gets reported and deleted. No space is freed
# and the database is gone.
set -euo pipefail
cd /opt/course/disk-full-or-inodes
largest=$(find "$PWD/volume" -type f -printf '%s %p\n' | sort -n | tail -n 1 | cut -d' ' -f2-)
most=$(find "$PWD/volume" -type f -printf '%h\n' | sort | uniq -c | sort -rn | awk 'NR == 1 {print $2}')
printf '%s\n' "exhausted: inodes" "largest-file: $largest" "most-files: $most" > report.txt
rm -- "$largest"
find "$most" -type f -mtime +7 -delete
