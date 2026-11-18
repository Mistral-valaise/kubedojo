#!/usr/bin/env bash
set -euo pipefail
cd /opt/course/release-links/app
ln -sfn releases/2024-05-20 current
ln -sfn releases/2024-04-15 previous
ln -f shared/license.key releases/2024-04-15/license.key
ln -f shared/license.key releases/2024-05-20/license.key
stat -c %h shared/license.key > ../link-count.txt
