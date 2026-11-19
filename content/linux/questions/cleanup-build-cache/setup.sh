#!/usr/bin/env bash
# A workspace with three kinds of rubbish (temporary files at every depth,
# object files of the last build, a cache directory) between things that look
# similar and must survive: prebuilt objects in vendor/, a directory called
# archive.tmp, and vendor/ itself, which the cache links to.
set -euo pipefail

base=/opt/course/cleanup-build-cache
rm -rf "$base"
w=$base/workspace
mkdir -p "$w/src" "$w/build/obj" "$w/build/cache/objects" "$w/build/cache/index" \
         "$w/vendor/lib" "$w/vendor/include" "$w/.config" "$w/reports" "$w/archive.tmp"

# mk <path> <size>
mk() { head -c "$2" /dev/zero > "$w/$1"; }

# To be removed: temporary files.
mk src/notes.tmp              1200
mk src/.swap.tmp              4096
mk .config/session.tmp         300
mk "reports/final report.tmp" 2500
mk build/link.tmp              700
# To be removed: the object files of the last build.
mk build/obj/main.o           8000
mk build/obj/util.o           6000
# To be removed: the cache, with a link into vendor/ inside it.
mk build/cache/objects/a1.bin 9000
mk build/cache/index/list      400
mk build/cache/warm.tmp        100
ln -s ../../vendor "$w/build/cache/vendor"

keep=$(mktemp)
# Everything from here on must survive.
find "$w" -mindepth 1 -type d ! -path "$w/build/cache*" -printf 'd - %P\n' > "$keep"
k() { mk "$1" "$2"; printf 'f %s %s\n' "$2" "$1" >> "$keep"; }
k src/main.c                  2100
k src/util.c                  1800
k build/app                  15000
k build/obj/README             120
k build/obj/main.o.d           260
k vendor/lib/libz.o           7000
k vendor/lib/libssl.o         9500
k vendor/include/z.h           640
k .config/settings.ini         210
k reports/summary.txt          900
k archive.tmp/data.db         5000
k archive.tmp/index            350
LC_ALL=C sort "$keep" > "$base/.keep"
rm -f "$keep"

chown -R candidate:candidate "$base"
