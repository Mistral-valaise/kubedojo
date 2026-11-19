#!/usr/bin/env bash
# expect: fail links untouched
# ln -sf without -n: `previous` is a link to a directory, so the new link is
# created inside releases/2024-03-01 and `previous` keeps its old target.
set -euo pipefail
cd /opt/course/release-links/app
ln -sf releases/2024-05-20 current
ln -sf releases/2024-04-15 previous
ln -f shared/license.key releases/2024-04-15/license.key
ln -f shared/license.key releases/2024-05-20/license.key
stat -c %h shared/license.key > ../link-count.txt
