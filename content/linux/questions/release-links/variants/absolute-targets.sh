#!/usr/bin/env bash
# expect: fail relocatable
# Everything works in place, but the symbolic links carry absolute paths and
# point back to the old location once app is copied to another host.
set -euo pipefail
app=/opt/course/release-links/app
ln -sfn "$app/releases/2024-05-20" "$app/current"
ln -sfn "$app/releases/2024-04-15" "$app/previous"
ln -f "$app/shared/license.key" "$app/releases/2024-04-15/license.key"
ln -f "$app/shared/license.key" "$app/releases/2024-05-20/license.key"
stat -c %h "$app/shared/license.key" > /opt/course/release-links/link-count.txt
