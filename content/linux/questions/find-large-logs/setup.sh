#!/usr/bin/env bash
# A tree of log files of different sizes and ages. Three of them are large
# (> 100 KiB), old (> 30 days) regular files named *.log; everything else is a
# decoy that fails exactly one of the tests.
set -euo pipefail

base=/opt/course/find-large-logs
rm -rf "$base"
mkdir -p "$base/logs/app/archive" "$base/logs/db" "$base/logs/web" "$base/logs/old.log"

# mk <path> <size> <age in days>
mk() {
  head -c "$2" /dev/zero > "$base/logs/$1"
  touch -d "$3 days ago" "$base/logs/$1"
}

# The answer.
mk app/server.log              300K  45
mk app/archive/server-2023.log 2M   400
mk "db/slow query.log"         150K  90

# Large and old, but not named *.log.
mk app/archive/server.log.1    400K 200
mk db/dump.sql                 500K 120
# Named *.log and old, but small. The second is exactly 100 KiB: not larger.
mk app/debug.log               40K   60
mk web/access.log              100K  75
# Named *.log and large, but recent.
mk web/error.log               300K   2
mk db/query.log                250K  10
# Large and old inside a directory that is itself called *.log.
mk old.log/data.bin            200K 300
touch -d "300 days ago" "$base/logs/old.log"
# A symbolic link to a matching file is not a regular file.
ln -s "$base/logs/app/server.log" "$base/logs/web/latest.log"

# What the guard check compares against: type, size and path of every entry.
(cd "$base/logs" && find . -printf '%y %s %p\n' | LC_ALL=C sort | sha256sum | cut -d' ' -f1) > "$base/.manifest"

# The learner works as candidate; the logs belong to root, as logs do.
chown candidate:candidate "$base"
