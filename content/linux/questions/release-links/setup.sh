#!/usr/bin/env bash
# An application directory with three releases. `current` is an absolute,
# dangling symbolic link, `previous` points to the wrong (existing) release,
# and of the three license.key files only one is a hard link to the shared
# file: one is a copy and one a symbolic link.
set -euo pipefail

base=/opt/course/release-links
rm -rf "$base"
app=$base/app
mkdir -p "$app/shared" "$app/releases"

printf 'LICENSE-KEY 7Q2M-91XA-KD40-LNX5\nissued-to: platform-team\n' > "$app/shared/license.key"
for r in 2024-03-01 2024-04-15 2024-05-20; do
  mkdir "$app/releases/$r"
  printf '#!/bin/sh\necho "app release %s"\n' "$r" > "$app/releases/$r/app.sh"
  chmod 755 "$app/releases/$r/app.sh"
done
ln    "$app/shared/license.key" "$app/releases/2024-03-01/license.key"   # as it should be
cp    "$app/shared/license.key" "$app/releases/2024-04-15/license.key"   # a copy: another inode
ln -s "$app/shared/license.key" "$app/releases/2024-05-20/license.key"   # a symbolic link

# 2024-05-02 was withdrawn and deleted; the link still names it.
ln -s "$app/releases/2024-05-02" "$app/current"
ln -s releases/2024-03-01 "$app/previous"

# For the guard: the names below releases/, and the content of the files.
(cd "$app" && { find releases -printf '%P\n' | LC_ALL=C sort; sha256sum shared/license.key releases/*/app.sh; } \
  | sha256sum | cut -d' ' -f1) > "$base/.manifest"

chown -R -h candidate:candidate "$base"
