#!/usr/bin/env bash
# expect: pass
# Another correct way: remove the old links first, let ln work out the
# relative target itself (-r), and create the hard links with cp -l.
set -euo pipefail
app=/opt/course/release-links/app
rm "$app/current" "$app/previous"
ln -s -r "$app/releases/2024-05-20" "$app/current"
ln -s -r "$app/releases/2024-04-15" "$app/previous"
for r in 2024-04-15 2024-05-20; do
  rm "$app/releases/$r/license.key"
  cp -l "$app/shared/license.key" "$app/releases/$r/license.key"
done
ls -l "$app/shared/license.key" | awk '{print $2}' > /opt/course/release-links/link-count.txt
